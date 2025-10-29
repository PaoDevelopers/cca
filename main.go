package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "cca.scfgs", "path to configuration file")
	flag.Parse()

	ctx := context.Background()

	var err error

	app := App{}

	// Config
	slog.Info("Loading configuration", slog.String("path", configPath))
	app.config, err = loadConfig(configPath)
	if err != nil {
		log.Fatalln(err)
	}

	// Database
	slog.Info("Connecting to the database")
	app.pool, err = pgxpool.New(ctx, app.config.Database)
	if err != nil {
		log.Fatalln(err)
	}
	app.queries = db.New(app.pool)
	version, err := app.queries.GetSchemaVersion(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	if version != 1 {
		log.Fatalln("Bad schema version")
	}

	// JWKS
	slog.Info("Fetching JWKS", slog.String("jwks", app.config.OIDC.JWKS))
	app.kf, err = keyfunc.NewDefault([]string{app.config.OIDC.JWKS})
	if err != nil {
		log.Fatalln(err)
	}

	// Templates
	slog.Info("Loading templates")
	err = app.admLoadTemplates()
	if err != nil {
		log.Fatalln(err)
	}

	// SSE broker
	slog.Info("Setting up SSE broker", slog.Int("buffer_length", app.config.SSEBuf))
	app.broker = NewBroker(app.config.SSEBuf)

	// Router
	slog.Info("Registering routes")
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", app.handleIndex)
	mux.HandleFunc("/auth", app.handleAuth)
	mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.Dir("admin_static"))))
	mux.HandleFunc("/admin/{$}", app.adminOnly("handleAdm", app.handleAdm))
	mux.HandleFunc("/admin/notify", app.adminOnly("handleAdmNotify", app.handleAdmNotify))
	mux.HandleFunc("/admin/periods", app.adminOnly("handleAdmPeriods", app.handleAdmPeriods))
	mux.HandleFunc("/admin/periods/new", app.adminOnly("handleAdmPeriodsNew", app.handleAdmPeriodsNew))
	mux.HandleFunc("/admin/periods/delete", app.adminOnly("handleAdmPeriodsDelete", app.handleAdmPeriodsDelete))
	mux.HandleFunc("/admin/categories", app.adminOnly("handleAdmCategories", app.handleAdmCategories))
	mux.HandleFunc("/admin/categories/new", app.adminOnly("handleAdmCategoriesNew", app.handleAdmCategoriesNew))
	mux.HandleFunc("/admin/categories/delete", app.adminOnly("handleAdmCategoriesDelete", app.handleAdmCategoriesDelete))
	mux.HandleFunc("/admin/grades", app.adminOnly("handleAdmGrades", app.handleAdmGrades))
	mux.HandleFunc("/admin/grades/new", app.adminOnly("handleAdmGradesNew", app.handleAdmGradesNew))
	mux.HandleFunc("/admin/grades/edit", app.adminOnly("handleAdmGradesEdit", app.handleAdmGradesEdit))
	mux.HandleFunc("/admin/grades/bulk-enabled-update", app.adminOnly("handleAdmGradesBulkEnabledUpdate", app.handleAdmGradesBulkEnabledUpdate))
	mux.HandleFunc("/admin/grades/delete", app.adminOnly("handleAdmGradesDelete", app.handleAdmGradesDelete))
	mux.HandleFunc("/admin/grades/new-requirement-group", app.adminOnly("handleAdmGradesNewRequirementGroup", app.handleAdmGradesNewRequirementGroup))
	mux.HandleFunc("/admin/grades/delete-requirement-group", app.adminOnly("handleAdmGradesDeleteRequirementGroup", app.handleAdmGradesDeleteRequirementGroup))
	mux.HandleFunc("/admin/courses", app.adminOnly("handleAdmCourses", app.handleAdmCourses))
	mux.HandleFunc("/admin/courses/new", app.adminOnly("handleAdmCoursesNew", app.handleAdmCoursesNew))
	mux.HandleFunc("/admin/courses/edit", app.adminOnly("handleAdmCoursesEdit", app.handleAdmCoursesEdit))
	mux.HandleFunc("/admin/courses/delete", app.adminOnly("handleAdmCoursesDelete", app.handleAdmCoursesDelete))
	mux.HandleFunc("/admin/courses/import", app.adminOnly("handleAdmCoursesImport", app.handleAdmCoursesImport))
	mux.HandleFunc("/admin/students", app.adminOnly("handleAdmStudents", app.handleAdmStudents))
	mux.HandleFunc("/admin/students/new", app.adminOnly("handleAdmStudentsNew", app.handleAdmStudentsNew))
	mux.HandleFunc("/admin/students/edit", app.adminOnly("handleAdmStudentsEdit", app.handleAdmStudentsEdit))
	mux.HandleFunc("/admin/students/delete", app.adminOnly("handleAdmStudentsDelete", app.handleAdmStudentsDelete))
	mux.HandleFunc("/admin/students/import", app.adminOnly("handleAdmStudentsImport", app.handleAdmStudentsImport))
	mux.HandleFunc("/admin/selections", app.adminOnly("handleAdmSelections", app.handleAdmSelections))
	mux.HandleFunc("/admin/selections/new", app.adminOnly("handleAdmSelectionsNew", app.handleAdmSelectionsNew))
	mux.HandleFunc("/admin/selections/edit", app.adminOnly("handleAdmSelectionsEdit", app.handleAdmSelectionsEdit))
	mux.HandleFunc("/admin/selections/delete", app.adminOnly("handleAdmSelectionsDelete", app.handleAdmSelectionsDelete))
	mux.HandleFunc("/admin/selections/import", app.adminOnly("handleAdmSelectionsImport", app.handleAdmSelectionsImport))
	mux.HandleFunc("/student", app.studentOnly("handleStu", app.handleStu))
	mux.Handle("/student/assets/", http.StripPrefix("/student/assets/", http.FileServer(http.Dir("frontend/dist/assets/"))))
	mux.HandleFunc("/student/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/student/assets/") {
			http.ServeFile(w, r, "./frontend/dist/index.html")
			return
		}
	})
	mux.HandleFunc("/student/api/events", app.studentOnly("handleStuAPIEvents", app.handleStuAPIEvents))
	mux.HandleFunc("/student/api/user_info", app.studentOnly("handleStuAPIInfo", app.handleStuAPIInfo))
	mux.HandleFunc("/student/api/courses", app.studentOnly("handleStuAPICourses", app.handleStuAPICourses))
	mux.HandleFunc("/student/api/periods", app.studentOnly("handleStuAPIPeriods", app.handleStuAPIPeriods))
	mux.HandleFunc("/student/api/categories", app.studentOnly("handleStuAPICategories", app.handleStuAPICategories))
	mux.HandleFunc("/student/api/grades", app.studentOnly("handleStuAPIGrades", app.handleStuAPIGrades))
	mux.HandleFunc("/student/api/my_selections", app.studentOnly("handleStuAPIMySelections", app.handleStuAPIMySelections))

	// Listen and serve
	slog.Info("Starting listener", slog.String("transport", app.config.Listen.Transport), slog.String("address", app.config.Listen.Address), slog.String("network", app.config.Listen.Network))
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
	slog.Info("Serving")
	(&http.Server{
		Handler: mux,
	}).Serve(l)
}
