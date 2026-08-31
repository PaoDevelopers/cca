package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/PaoDevelopers/cca/internal/config"
	"github.com/PaoDevelopers/cca/internal/db"
	"github.com/PaoDevelopers/cca/ui"
	"github.com/jackc/pgx/v5/pgxpool"
)

// readHeaderTimeout is the only bounded phase of a request: bodies are
// bounded per-handler and the event websockets are long-lived.
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout bounds how long an in-flight request may hold up a
// shutdown. The event websockets are hijacked, so Shutdown does not
// wait for them; this is only for ordinary requests.
//
// Longer than writeTimeout on purpose, with room to spare. A write
// runs on a context detached from the request precisely so that a
// committed statement is always followed by its broadcast; cutting
// the process off at fifteen seconds would take that guarantee back
// for the slowest writes — the pool closes underneath a transaction
// that may already have committed, and the handler dies between the
// commit and the announcement. That is the exact failure writeCtx
// exists to prevent, reintroduced at the one moment nobody is
// watching for it.
//
// The margin is for the rest of the handler: reading the result,
// building the response, telling the hub.
const shutdownTimeout = writeTimeout + 15*time.Second

var (
	errUnknownTransport      = errors.New(`unknown listen transport, expected "plain" or "tls"`)
	errSchemaVersionMismatch = errors.New("unexpected database schema version")
)

// The paths are compile-time constants, so failure is a build defect.
func mustSubFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}

	return sub
}

// immutable marks a response as cacheable forever. Only correct for
// content-addressed names: vite puts a hash of the contents in every
// filename under assets/, so a changed file is a changed URL. The
// files under static/ are *not* hashed and must not use this.
func immutable(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

// Run wires up and serves the entire application until it is asked to
// stop, or fails. It is the only place that ends the process.
func Run(configPath string) {
	// Cancelled on the first SIGINT or SIGTERM. Startup work uses this
	// too, so a signal during startup stops it rather than being
	// noticed only once it finishes.
	//
	// Nothing here is deferred: the failure paths end the process, and
	// a deferred cleanup would not run.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	app, err := newServer(ctx, configPath)
	if err != nil {
		fatal("start", err)
	}

	go app.wsHub.Run(ctx)

	// After the hub, so a boundary that has already passed has
	// somewhere to broadcast to.
	app.windowTimer = newWindowTimer(ctx.Done())
	app.rearmWindowTimer(ctx)

	listener, err := listen(ctx, app.config)
	if err != nil {
		fatal("listen", err)
	}

	app.serve(ctx, stop, listener)
}

// errNoJWKSKeys reports that the identity provider's key set was
// reachable in form but empty in substance.
var errNoJWKSKeys = errors.New("the key set served no keys")

// fatal ends the process over a startup error that leaves nothing to
// serve.
//
// Not log.Fatalln: slog.SetDefault redirects the standard log package
// into the JSON handler at LevelInfo, on stdout. So the one line an
// operator most needs — the service will not start — arrived at the
// level everything else routine arrives at, on the stream nobody
// alerts on, with the error text as the message, so there was not even
// a constant to match on. It goes to stderr at ERROR under a stable
// name instead.
func fatal(stage string, err error) {
	//exhaustruct:ignore
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	slog.New(h).Error(logMsgStartupFailed,
		slog.String("stage", stage),
		slog.String("error", err.Error()))
	os.Exit(1)
}

// newServer brings up everything a request needs, in dependency order.
func newServer(ctx context.Context, configPath string) (*Server, error) {
	// Every field is assigned below, before the server is used.
	//exhaustruct:ignore
	app := &Server{}

	var err error

	slog.Info(logMsgStartupConfigLoad, slog.String("path", configPath))

	app.config, err = config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	slog.Info(logMsgStartupDBConnect)

	app.pool, err = openDatabase(ctx, app.config)
	if err != nil {
		return nil, err
	}

	app.queries = db.New(app.pool)

	if err := checkSchemaVersion(ctx, app.queries); err != nil {
		return nil, err
	}

	app.sessionKey, err = newSessionKey(app.config.Session.Key)
	if err != nil {
		return nil, fmt.Errorf("session key: %w", err)
	}

	slog.Info(logMsgStartupJWKSFetch, slog.String("jwks", app.config.OIDC.JWKS))

	keys, err := keyfunc.NewDefault([]string{app.config.OIDC.JWKS})
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}

	// NewDefault does not fail when the first fetch fails: it logs and
	// hands back a working Keyfunc holding no keys. Serving on that is
	// the worst outcome available — every probe green, every sign-in
	// refused as an unparsable token, and the only trace a single line
	// at boot. So the keys are counted rather than assumed, and a
	// process that cannot authenticate anybody does not start.
	loaded, err := keys.Storage().KeyReadAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read jwks: %w", err)
	}

	if len(loaded) == 0 {
		return nil, fmt.Errorf("fetch jwks from %s: %w",
			app.config.OIDC.JWKS, errNoJWKSKeys)
	}

	app.verifyKey = keys.Keyfunc

	slog.Info(logMsgStartupWebsocketSetup)

	app.wsHub = NewWebSocketHub(app.queries)

	return app, nil
}

// openDatabase applies the pool settings from the configuration.
//
// The "if the value is positive" guards below are belt and braces
// rather than the mechanism: config.validate already rejects anything
// at or below the floors, so by the time this runs every setting but
// min_conns has a value. They are kept because a floor and a guard
// disagreeing is a worse failure than either alone.
func openDatabase(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	dbCfg := cfg.Database

	poolConfig, err := pgxpool.ParseConfig(dbCfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
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

	// v_courses returns legal_sex[], which pgx cannot decode without
	// the type's OID; every connection to a cca database loads it.
	poolConfig.AfterConnect = db.RegisterTypes

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

// There are no automatic migrations, so refusing to start is how an
// administrator finds out they have not applied one.
func checkSchemaVersion(ctx context.Context, queries *db.Queries) error {
	version, err := queries.GetSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version != db.ExpectedSchemaVersion {
		return fmt.Errorf("%w: database has %d, this build expects %d",
			errSchemaVersionMismatch, version, db.ExpectedSchemaVersion)
	}

	return nil
}

// router registers every route. Kept whole and in one place, because
// the order and precedence of these patterns is the routing.
func (app *Server) router() *http.ServeMux {
	slog.Info(logMsgStartupRoutesRegister)

	mux := http.NewServeMux()

	// Every API route is registered through this, so the catch-alls
	// below know which methods each path accepts.
	routes := newAPIRoutes()

	studentAPI := func(pattern string, name string, handler func(http.ResponseWriter, *http.Request, *UserInfoStudent)) {
		routes.record(pattern)
		mux.HandleFunc(pattern, app.studentAPI(name, handler))
	}

	mux.HandleFunc("GET /{$}", app.handleIndex)
	mux.HandleFunc("GET /api/session", app.handleSession)
	// The portal builds at base "/", so its assets are at the root
	// rather than under an area prefix. No StripPrefix: the sub-FS is
	// rooted at portal/dist, where assets/ already is.
	mux.Handle("/assets/", immutable(http.FileServerFS(mustSubFS(ui.Dist, "portal/dist"))))
	mux.HandleFunc("GET /healthz", app.handleHealthz)
	mux.HandleFunc("GET /readyz", app.handleReadyz)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFS, "static/favicon.ico")
	})
	mux.HandleFunc("POST /auth", app.handleAuth)
	app.registerAdminAPI(mux, routes)

	// More specific than "/admin/" and less specific than every route
	// above, so it catches exactly what nothing else claimed.
	mux.HandleFunc("/admin/api/", app.apiCatchAll(routes))
	mux.Handle("/admin/static/", http.StripPrefix("/admin/", http.FileServerFS(staticFS)))
	mux.Handle("/admin/assets/", immutable(http.StripPrefix("/admin/", http.FileServerFS(mustSubFS(ui.Dist, "admin/dist")))))
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
	})
	mux.HandleFunc("/admin/", app.spaHandler(roleAdmin, "admin/dist/index.html", func(r *http.Request) error {
		_, err := app.authenticateAdmin(r)

		return err
	}))
	mux.HandleFunc("POST /admin/logout", app.handleLogout(roleAdmin))
	mux.HandleFunc("/student", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/student/", http.StatusSeeOther)
	})
	mux.Handle("/student/assets/", immutable(http.StripPrefix("/student/", http.FileServerFS(mustSubFS(ui.Dist, "student/dist")))))
	mux.HandleFunc("/student/", app.spaHandler(roleStudent, "student/dist/index.html", func(r *http.Request) error {
		_, err := app.authenticateStudent(r)

		return err
	}))
	mux.HandleFunc("POST /student/logout", app.handleLogout(roleStudent))
	studentAPI("/student/api/events", "handleStuAPIEvents", app.handleStuAPIEvents)
	studentAPI("/student/api/user_info", "handleStuAPIInfo", app.handleStuAPIInfo)
	studentAPI("/student/api/courses", "handleStuAPICourses", app.handleStuAPICourses)
	studentAPI("/student/api/periods", "handleStuAPIPeriods", app.handleStuAPIPeriods)
	studentAPI("/student/api/categories", "handleStuAPICategories", app.handleStuAPICategories)
	studentAPI("/student/api/eligibility", "handleStuAPIEligibility", app.handleStuAPIEligibility)
	studentAPI("/student/api/grades", "handleStuAPIGrades", app.handleStuAPIGrades)
	studentAPI("/student/api/my_enrollments", "handleStuAPIMyEnrollments", app.handleStuAPIMyEnrollments)

	mux.HandleFunc("/student/api/", app.apiCatchAll(routes))

	return mux
}

func listen(ctx context.Context, cfg config.Config) (net.Listener, error) {
	slog.Info(logMsgStartupListenerStart,
		slog.String("transport", cfg.Listen.Transport),
		slog.String("address", cfg.Listen.Address),
		slog.String("network", cfg.Listen.Network))

	//exhaustruct:ignore
	lc := net.ListenConfig{}

	switch cfg.Listen.Transport {
	case "plain":
		l, err := lc.Listen(ctx, cfg.Listen.Network, cfg.Listen.Address)
		if err != nil {
			return nil, fmt.Errorf("listen plain: %w", err)
		}

		return l, nil
	case "tls":
		c, err := tls.LoadX509KeyPair(cfg.Listen.TLS.Cert, cfg.Listen.TLS.Key)
		if err != nil {
			return nil, fmt.Errorf("load TLS keys: %w", err)
		}

		//exhaustruct:ignore
		tc := tls.Config{
			Certificates: []tls.Certificate{c},
			MinVersion:   tls.VersionTLS13,
		}

		inner, err := lc.Listen(ctx, cfg.Listen.Network, cfg.Listen.Address)
		if err != nil {
			return nil, fmt.Errorf("listen TLS: %w", err)
		}

		return tls.NewListener(inner, &tc), nil
	default:
		// Without this, an unrecognised transport would leave the
		// listener nil and surface as a nil-pointer panic inside Serve,
		// which says nothing about the typo that caused it.
		return nil, fmt.Errorf("%w: %q", errUnknownTransport, cfg.Listen.Transport)
	}
}

// serve runs until the context is cancelled by a signal, then stops
// accepting, lets in-flight requests finish, and closes what is open.
// It returns only on a clean shutdown; a serve failure ends the
// process.
func (app *Server) serve(ctx context.Context, stop context.CancelFunc, listener net.Listener) {
	slog.Info(logMsgStartupServing)

	//exhaustruct:ignore
	server := &http.Server{
		Handler:           securityHeaders(app.crossOriginProtection().Handler(app.router())),
		ReadHeaderTimeout: readHeaderTimeout,
		ErrorLog:          newHTTPServerErrorLogger(),
	}

	serveErr := make(chan error, 1)

	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		// Serve returns ErrServerClosed only once Shutdown has been
		// called, which cannot have happened yet.
		slog.Error(logMsgHTTPServerServeFailure, slog.Any("error", err))
		os.Exit(1)
	case <-ctx.Done():
		slog.Info(logMsgShutdownSignal)
	}

	// Restores the default disposition, so a second signal from an
	// impatient operator kills the process rather than being swallowed.
	stop()

	// WithoutCancel, not Background: the parent is already cancelled —
	// that is why we are here — but its values still apply.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error(logMsgShutdownTimeout, slog.Any("error", err))
	}

	app.wsHub.CloseAll()
	app.pool.Close()
	slog.Info(logMsgShutdownComplete)
}
