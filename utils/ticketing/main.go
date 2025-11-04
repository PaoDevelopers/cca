package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		listenAddr     = flag.String("listen", ":8080", "HTTP listen address")
		dbURL          = flag.String("db", "postgresql:///?host=/var/run/postgresql&dbname=ccatickets", "PostgreSQL connection string")
		staticDir      = flag.String("static", "", "Directory for static files")
		templateDir    = flag.String("templates", "templates", "Directory containing HTML templates")
		attachmentsDir = flag.String("attachments", "attachments", "Directory for storing attachments")
		inviteCode     = flag.String("invite", "", "Invite code required for registration")
		logLevel       = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(newJSONHandler(os.Stdout, parseLevel(*logLevel)))
	logger.Info("starting ticketing server", "listen", *listenAddr)

	pool, err := connectDatabase(ctx, *dbURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := NewStore(pool)
	if err := store.CheckVersion(ctx); err != nil {
		logger.Error("schema version check failed", "error", err)
		os.Exit(1)
	}

	templates, err := LoadTemplates(*templateDir)
	if err != nil {
		logger.Error("failed to load templates", "error", err)
		os.Exit(1)
	}

	attachments, err := NewFileStorage(*attachmentsDir)
	if err != nil {
		logger.Error("failed to initialise attachments storage", "error", err)
		os.Exit(1)
	}

	code := strings.TrimSpace(*inviteCode)
	if code == "" {
		logger.Error("invite code must be provided via -invite")
		os.Exit(1)
	}

	sessionManager := &SessionManager{
		Store:           store,
		Logger:          logger,
		CookieName:      "session_token",
		SessionDuration: 24 * time.Hour,
	}

	server := &Server{
		Store:        store,
		Templates:    templates,
		Logger:       logger,
		SessionMaker: sessionManager,
		Attachments:  attachments,
		InviteCode:   code,
		StaticDir:    strings.TrimSpace(*staticDir),
	}

	httpServer := newHTTPServer(*listenAddr, server.Handler())

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	logger.Info("server stopped")
}

func connectDatabase(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MinConns = 5
	cfg.MaxConns = 20
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func newJSONHandler(w io.Writer, level slog.Leveler) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	return slog.NewJSONHandler(w, opts)
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}
