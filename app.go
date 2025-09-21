package main

import (
	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/db"
)

type App struct {
	config  Config
	pool    *pgxpool.Pool
	queries *db.Queries
	kf      keyfunc.Keyfunc
	// sseHub  *SSEHub
}
