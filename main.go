// YK Pao School Co-curricular Activities Selection System Backend
package main

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

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	var configPath string
	flag.StringVar(&configPath, "c", "cca.scfgs", "path to configuration file")
	flag.Parse()

	ctx := context.Background()

	var err error

	app := App{}

	// Config
	slog.Info(logMsgStartupConfigLoad, slog.String("path", configPath))
	app.config, err = loadConfig(configPath)
	if err != nil {
		log.Fatalln(err)
	}

	if app.config.DP42IK.KeyB64 != "" {
		app.dp42ikKey, err = base64.StdEncoding.DecodeString(app.config.DP42IK.KeyB64)
		if err != nil {
			log.Fatalf("decode dp42ik key: %v", err)
		}
		if len(app.dp42ikKey) != 32 {
			log.Fatalf("decode dp42ik key: got %d bytes, want 32", len(app.dp42ikKey))
		}
		if app.config.DP42IK.ServiceID == "" {
			log.Fatalln("dp42ik service_id is required when key_b64 is set")
		}
		if app.config.DP42IK.KeyID < 0 || app.config.DP42IK.KeyID > 255 {
			log.Fatalln(fmt.Errorf("dp42ik key_id must be between 0 and 255: %d", app.config.DP42IK.KeyID))
		}
	}

	// Database
	slog.Info(logMsgStartupDBConnect)
	dbCfg := app.config.Database
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
	app.pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := app.pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	app.queries = db.New(app.pool)
	version, err := app.queries.GetSchemaVersion(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	if version != 2 {
		log.Fatalln("Bad schema version")
	}

	// Authentication provider. Test mode intentionally skips the external JWKS
	// fetch so local development can run completely offline.
	if app.config.TestAuth.Enabled {
		slog.Warn("TEST AUTHENTICATION IS ENABLED", slog.Bool("allow_remote", app.config.TestAuth.AllowRemote))
	} else {
		slog.Info(logMsgStartupJWKSFetch, slog.String("jwks", app.config.OIDC.JWKS))
		app.kf, err = keyfunc.NewDefault([]string{app.config.OIDC.JWKS})
		if err != nil {
			log.Fatalln(err)
		}
	}

	// WebSocket hub
	slog.Info(logMsgStartupWebsocketSetup)
	app.wsHub = NewWebSocketHub()
	go app.wsHub.Run()
	go app.runGradeSelectionScheduler(ctx)

	// Router
	slog.Info(logMsgStartupRoutesRegister)
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", app.handleIndex)
	mux.HandleFunc("/auth", app.handleAuth)
	mux.HandleFunc("/dp42ik", app.handleDP42IK)
	mux.Handle("/assets/", frontendAssetsHandler())

	// Versioned JSON API used by both React application areas.
	mux.HandleFunc("/api/v1/session", app.handleAPISession)
	mux.HandleFunc("/api/v1/test-auth", app.handleAPITestAuth)
	mux.HandleFunc("/api/v1/student/bootstrap", app.apiStudentOnly("handleAPIStudentBootstrap", app.handleAPIStudentBootstrap))
	mux.HandleFunc("/api/v1/student/courses", app.apiStudentOnly("handleAPIStudentCourses", app.handleAPIStudentCourses))
	mux.HandleFunc("/api/v1/student/periods", app.apiStudentOnly("handleAPIStudentPeriods", app.handleAPIStudentPeriods))
	mux.HandleFunc("/api/v1/student/grades", app.apiStudentOnly("handleAPIStudentGrades", app.handleAPIStudentGrades))
	mux.HandleFunc("/api/v1/student/selections", app.apiStudentOnly("handleAPIStudentSelections", app.handleAPIStudentSelections))
	mux.HandleFunc("/api/v1/student/events", app.apiStudentOnly("handleStuAPIEvents", app.handleStuAPIEvents))
	mux.HandleFunc("/api/v1/admin/dashboard", app.apiAdminOnly("handleAPIAdminDashboard", app.handleAPIAdminDashboard))
	mux.HandleFunc("/api/v1/admin/bootstrap", app.apiAdminOnly("handleAPIAdminBootstrap", app.handleAPIAdminBootstrap))
	mux.HandleFunc("/api/v1/admin/categories", app.apiAdminOnly("handleAPIAdminCategories", app.handleAPIAdminCategories))
	mux.HandleFunc("/api/v1/admin/categories/{id}", app.apiAdminOnly("handleAPIAdminCategory", app.handleAPIAdminCategory))
	mux.HandleFunc("/api/v1/admin/periods", app.apiAdminOnly("handleAPIAdminPeriods", app.handleAPIAdminPeriods))
	mux.HandleFunc("/api/v1/admin/grades", app.apiAdminOnly("handleAPIAdminGrades", app.handleAPIAdminGrades))
	mux.HandleFunc("/api/v1/admin/grades/{grade}", app.apiAdminOnly("handleAPIAdminGrade", app.handleAPIAdminGrade))
	mux.HandleFunc("/api/v1/admin/grade-access", app.apiAdminOnly("handleAPIAdminGradeAccess", app.handleAPIAdminGradeAccess))
	mux.HandleFunc("/api/v1/admin/grade-schedules", app.apiAdminOnly("handleAPIAdminGradeSchedules", app.handleAPIAdminGradeSchedules))
	mux.HandleFunc("/api/v1/admin/grade-schedules/{batch_id}", app.apiAdminOnly("handleAPIAdminGradeSchedule", app.handleAPIAdminGradeSchedule))
	mux.HandleFunc("/api/v1/admin/grades/{grade}/requirement-groups", app.apiAdminOnly("handleAPIAdminRequirementGroups", app.handleAPIAdminRequirementGroups))
	mux.HandleFunc("/api/v1/admin/grades/{grade}/requirement-groups/{id}", app.apiAdminOnly("handleAPIAdminRequirementGroup", app.handleAPIAdminRequirementGroup))
	mux.HandleFunc("/api/v1/admin/courses", app.apiAdminOnly("handleAPIAdminCourses", app.handleAPIAdminCourses))
	mux.HandleFunc("/api/v1/admin/courses/{id}", app.apiAdminOnly("handleAPIAdminCourse", app.handleAPIAdminCourse))
	mux.HandleFunc("/api/v1/admin/students", app.apiAdminOnly("handleAPIAdminStudents", app.handleAPIAdminStudents))
	mux.HandleFunc("/api/v1/admin/students/{id}", app.apiAdminOnly("handleAPIAdminStudent", app.handleAPIAdminStudent))
	mux.HandleFunc("/api/v1/admin/selections", app.apiAdminOnly("handleAPIAdminSelections", app.handleAPIAdminSelections))
	mux.HandleFunc("/api/v1/admin/selections/{student_id}/{course_id}", app.apiAdminOnly("handleAPIAdminSelection", app.handleAPIAdminSelection))
	mux.HandleFunc("/api/v1/admin/notifications", app.apiAdminOnly("handleAPIAdminNotifications", app.handleAPIAdminNotifications))
	mux.HandleFunc("/api/v1/admin/reset", app.apiAdminOnly("handleAPIAdminReset", app.handleAPIAdminReset))

	// Tabular transfers remain available to the React data-management page. All
	// mutating uploads use the same authenticated, same-origin guard as the JSON
	// API. The former template CRUD routes are intentionally not registered.
	mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.Dir("admin_static"))))
	mux.HandleFunc("/admin/data/examples/{kind}", app.apiAdminOnly("handleAdmDataExample", app.handleAdmDataExample))
	mux.HandleFunc("/admin/courses/import", app.apiAdminOnly("handleAdmCoursesImport", app.handleAdmCoursesImport))
	mux.HandleFunc("/admin/students/import", app.apiAdminOnly("handleAdmStudentsImport", app.handleAdmStudentsImport))
	mux.HandleFunc("/admin/selections/export", app.apiAdminOnly("handleAdmSelectionsExport", app.handleAdmSelectionsExport))
	mux.HandleFunc("/admin/selections/import", app.apiAdminOnly("handleAdmSelectionsImport", app.handleAdmSelectionsImport))
	mux.HandleFunc("/student", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/student/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/student/{path...}", app.studentOnlyPlain("studentFrontend", serveFrontendIndex))
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/admin/{path...}", app.adminOnlyPlain("adminFrontend", serveFrontendIndex))
	mux.HandleFunc("/test-login", app.handleTestAuthFrontend)
	mux.HandleFunc("/test-login/{path...}", app.handleTestAuthFrontend)

	// Listen and serve
	slog.Info(logMsgStartupListenerStart, slog.String("transport", app.config.Listen.Transport), slog.String("address", app.config.Listen.Address), slog.String("network", app.config.Listen.Network))
	var l net.Listener
	switch app.config.Listen.Transport {
	case "plain":
		l, err = net.Listen(app.config.Listen.Network, app.config.Listen.Address)
		if err != nil {
			log.Fatalf("Cannot listen plain: %v\n", err)
		}
	case "tls":
		c, err := tls.LoadX509KeyPair(app.config.Listen.TLS.Cert, app.config.Listen.TLS.Key)
		if err != nil {
			log.Fatalf("Cannot load TLS keys: %v\n", err)
		}

		tc := tls.Config{
			Certificates: []tls.Certificate{c},
			MinVersion:   tls.VersionTLS13,
		}

		l, err = tls.Listen(app.config.Listen.Network, app.config.Listen.Address, &tc)
		if err != nil {
			log.Fatalf("Cannot listen TLS: %v\n", err)
		}
	}
	slog.Info(logMsgStartupServing)
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second * time.Duration(10),
		ErrorLog:          newHTTPServerErrorLogger(),
	}
	if err := server.Serve(l); err != nil {
		slog.Error(logMsgHTTPServerServeFailure, slog.Any("error", err))
		os.Exit(1)
	}
}
