package main

import (
	"html/template"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.sr.ht/~runxiyu/cca/db"
)

type App struct {
	config  Config
	pool    *pgxpool.Pool
	queries *db.Queries
	kf      keyfunc.Keyfunc
	admTmpl map[string]*template.Template
	wsHub   *WebSocketHub
}
