package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/PaoDevelopers/cca/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "cca.scfgs", "path to configuration file")
	flag.Parse()
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalln(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, config.Database)
	if err != nil {
		log.Fatalln(err)
	}
	queries := db.New(pool)
	version, err := queries.GetSchemaVersion(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	if version != 1 {
		log.Fatalln("Bad schema version")
	}
	app := App{
		config:  config,
		pool:    pool,
		queries: queries,
	}
	err = app.setupJWKS()
	if err != nil {
		log.Fatalln(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/auth", app.handleAuth)
	mux.HandleFunc("/userinfo", app.handleUserInfo)

	log.Fatal(http.ListenAndServe(config.Listen.Address, mux))
	// TODO
}
