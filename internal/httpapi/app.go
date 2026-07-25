package httpapi

import (
	"git.sr.ht/~runxiyu/cca/internal/config"
	"git.sr.ht/~runxiyu/cca/internal/courses"
	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// App owns the HTTP dependencies and background services.
type App struct {
	config        config.Config
	pool          *pgxpool.Pool
	queries       *db.Queries
	kf            keyfunc.Keyfunc
	dp42ikKey     []byte
	wsHub         *WebSocketHub
	courseService *courses.Service
}

// New constructs an HTTP application from initialized server dependencies.
func New(
	cfg config.Config,
	pool *pgxpool.Pool,
	queries *db.Queries,
	kf keyfunc.Keyfunc,
	dp42ikKey []byte,
) *App {
	return &App{
		config:        cfg,
		pool:          pool,
		queries:       queries,
		kf:            kf,
		dp42ikKey:     dp42ikKey,
		wsHub:         NewWebSocketHub(),
		courseService: courses.NewService(pool, queries),
	}
}
