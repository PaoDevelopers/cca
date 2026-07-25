// Package app assembles and runs the CCA backend service.
package app

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"git.sr.ht/~runxiyu/cca/internal/config"
	"git.sr.ht/~runxiyu/cca/internal/httpapi"
	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	logMsgStartupConfigLoad      = "startup.config.load"
	logMsgStartupDBConnect       = "startup.db.connect"
	logMsgStartupJWKSFetch       = "startup.jwks.fetch"
	logMsgStartupWebsocketSetup  = "startup.websocket.setup" // #nosec G101 -- structured log event, not a credential.
	logMsgStartupRoutesRegister  = "startup.routes.register"
	logMsgStartupListenerStart   = "startup.listener.start"
	logMsgStartupServing         = "startup.server.serve"
	logMsgHTTPServerServeFailure = "http.server.serve_failure"
)

// Run starts the configured CCA backend and blocks while it serves requests.
func Run() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	var configPath string
	flag.StringVar(&configPath, "c", "cca.scfgs", "path to configuration file")
	flag.Parse()

	ctx := context.Background()

	var (
		err       error
		cfg       config.Config
		dp42ikKey []byte
		pool      *pgxpool.Pool
		queries   *db.Queries
		kf        keyfunc.Keyfunc
	)

	// Config
	slog.Info(logMsgStartupConfigLoad, slog.String("path", configPath))
	cfg, err = config.Load(configPath)
	if err != nil {
		log.Fatalln(err)
	}

	if cfg.DP42IK.KeyB64 != "" {
		dp42ikKey, err = base64.StdEncoding.DecodeString(cfg.DP42IK.KeyB64)
		if err != nil {
			log.Fatalf("decode dp42ik key: %v", err)
		}
		if len(dp42ikKey) != 32 {
			log.Fatalf("decode dp42ik key: got %d bytes, want 32", len(dp42ikKey))
		}
		if cfg.DP42IK.ServiceID == "" {
			log.Fatalln("dp42ik service_id is required when key_b64 is set")
		}
		if cfg.DP42IK.KeyID < 0 || cfg.DP42IK.KeyID > 255 {
			log.Fatalln(fmt.Errorf("dp42ik key_id must be between 0 and 255: %d", cfg.DP42IK.KeyID))
		}
	}

	// Database
	slog.Info(logMsgStartupDBConnect)
	dbCfg := cfg.Database
	poolConfig, err := pgxpool.ParseConfig(dbCfg.URL)
	if err != nil {
		log.Fatalf("parse database config: %v", err)
	}
	if dbCfg.MaxConns > 0 {
		poolConfig.MaxConns = dbCfg.MaxConns
	}
	if dbCfg.MinConns > 0 {
		poolConfig.MinConns = dbCfg.MinConns
	}
	if dbCfg.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = dbCfg.MaxConnLifetime
	}
	if dbCfg.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = dbCfg.MaxConnIdleTime
	}
	if dbCfg.HealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = dbCfg.HealthCheckPeriod
	}
	if dbCfg.ConnectTimeout > 0 {
		poolConfig.ConnConfig.ConnectTimeout = dbCfg.ConnectTimeout
	}
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	// The student catalogue is a short, set-oriented read whose conservative
	// planner estimate otherwise crosses PostgreSQL's JIT threshold. Compiling
	// that query on every request costs far more than executing it.
	poolConfig.ConnConfig.RuntimeParams["jit"] = "off"
	pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	queries = db.New(pool)
	version, err := queries.GetSchemaVersion(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	if version != 2 {
		log.Fatalln("Bad schema version")
	}

	// Authentication provider. Test mode intentionally skips the external JWKS
	// fetch so local development can run completely offline.
	if cfg.TestAuth.Enabled {
		slog.Warn("TEST AUTHENTICATION IS ENABLED", slog.Bool("allow_remote", cfg.TestAuth.AllowRemote))
	} else {
		slog.Info(logMsgStartupJWKSFetch, slog.String("jwks", cfg.OIDC.JWKS))
		kf, err = keyfunc.NewDefault([]string{cfg.OIDC.JWKS})
		if err != nil {
			log.Fatalln(err)
		}
	}

	slog.Info(logMsgStartupWebsocketSetup)
	serverApp := httpapi.New(cfg, pool, queries, kf, dp42ikKey)
	serverApp.StartBackground(ctx)

	slog.Info(logMsgStartupRoutesRegister)
	mux := serverApp.Handler()

	// Listen and serve
	slog.Info(logMsgStartupListenerStart, slog.String("transport", cfg.Listen.Transport), slog.String("address", cfg.Listen.Address), slog.String("network", cfg.Listen.Network))
	var l net.Listener
	switch cfg.Listen.Transport {
	case "plain":
		l, err = net.Listen(cfg.Listen.Network, cfg.Listen.Address)
		if err != nil {
			log.Fatalf("Cannot listen plain: %v\n", err)
		}
	case "tls":
		c, err := tls.LoadX509KeyPair(cfg.Listen.TLS.Cert, cfg.Listen.TLS.Key)
		if err != nil {
			log.Fatalf("Cannot load TLS keys: %v\n", err)
		}

		tc := tls.Config{
			Certificates: []tls.Certificate{c},
			MinVersion:   tls.VersionTLS13,
		}

		l, err = tls.Listen(cfg.Listen.Network, cfg.Listen.Address, &tc)
		if err != nil {
			log.Fatalf("Cannot listen TLS: %v\n", err)
		}
	}
	slog.Info(logMsgStartupServing)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second * time.Duration(10),
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          httpapi.NewHTTPServerErrorLogger(),
	}
	if err := server.Serve(l); err != nil {
		slog.Error(logMsgHTTPServerServeFailure, slog.Any("error", err))
		os.Exit(1)
	}
}
