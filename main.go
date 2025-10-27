package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"

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
	log.Println("Loading configuration at " + configPath)
	app.config, err = loadConfig(configPath)
	if err != nil {
		log.Fatalln(err)
	}

	// Database
	log.Println("Connecting to the database")
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
	log.Println("Fetching JWKS")
	app.kf, err = keyfunc.NewDefault([]string{app.config.OIDC.JWKS})
	if err != nil {
		log.Fatalln(err)
	}

	// Templates
	log.Println("Loading templates")
	err = app.admLoadTemplates()
	if err != nil {
		log.Fatalln(err)
	}

	// Router
	log.Println("Registering routes")
	mux := http.NewServeMux()
	mux.HandleFunc("/{$}", app.handleIndex)
	mux.HandleFunc("/auth", app.handleAuth)
	mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.Dir("admin-static"))))
	mux.HandleFunc("/admin", app.adminOnly(app.handleAdm))
	mux.HandleFunc("/admin/periods", app.adminOnly(app.handleAdmPeriods))
	mux.HandleFunc("/admin/periods/new", app.adminOnly(app.handleAdmPeriodsNew))
	mux.HandleFunc("/admin/periods/delete", app.adminOnly(app.handleAdmPeriodsDelete))
	mux.HandleFunc("/admin/categories", app.adminOnly(app.handleAdmCategories))
	mux.HandleFunc("/admin/categories/new", app.adminOnly(app.handleAdmCategoriesNew))
	mux.HandleFunc("/admin/categories/delete", app.adminOnly(app.handleAdmCategoriesDelete))
	mux.HandleFunc("/admin/grades", app.adminOnly(app.handleAdmGrades))
	mux.HandleFunc("/admin/grades/new", app.adminOnly(app.handleAdmGradesNew))
	mux.HandleFunc("/admin/grades/edit", app.adminOnly(app.handleAdmGradesEdit))
	mux.HandleFunc("/admin/grades/bulk-enabled-update", app.adminOnly(app.handleAdmGradesBulkEnabledUpdate))
	mux.HandleFunc("/admin/grades/delete", app.adminOnly(app.handleAdmGradesDelete))
	mux.HandleFunc("/admin/grades/new-requirement-group", app.adminOnly(app.handleAdmGradesNewRequirementGroup))
	mux.HandleFunc("/admin/grades/delete-requirement-group", app.adminOnly(app.handleAdmGradesDeleteRequirementGroup))
	mux.HandleFunc("/admin/courses", app.adminOnly(app.handleAdmCourses))
	mux.HandleFunc("/admin/courses/new", app.adminOnly(app.handleAdmCoursesNew))
	mux.HandleFunc("/admin/courses/edit", app.adminOnly(app.handleAdmCoursesEdit))
	mux.HandleFunc("/admin/courses/delete", app.adminOnly(app.handleAdmCoursesDelete))
	mux.HandleFunc("/admin/courses/import", app.adminOnly(app.handleAdmCoursesImport))
	mux.HandleFunc("/admin/students", app.adminOnly(app.handleAdmStudents))
	mux.HandleFunc("/admin/students/new", app.adminOnly(app.handleAdmStudentsNew))
	mux.HandleFunc("/admin/students/edit", app.adminOnly(app.handleAdmStudentsEdit))
	mux.HandleFunc("/admin/students/delete", app.adminOnly(app.handleAdmStudentsDelete))
	mux.HandleFunc("/admin/students/import", app.adminOnly(app.handleAdmStudentsImport))
	mux.HandleFunc("/admin/selections", app.adminOnly(app.handleAdmSelections))
	mux.HandleFunc("/admin/selections/new", app.adminOnly(app.handleAdmSelectionsNew))
	mux.HandleFunc("/admin/selections/edit", app.adminOnly(app.handleAdmSelectionsEdit))
	mux.HandleFunc("/admin/selections/delete", app.adminOnly(app.handleAdmSelectionsDelete))
	mux.HandleFunc("/student/api/userinfo", app.studentOnly(app.handleStuAPIInfo))

	// Listen and serve
	log.Println("Starting listener")
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
	log.Println("Serving")
	(&http.Server{
		Handler: mux,
	}).Serve(l)
}
