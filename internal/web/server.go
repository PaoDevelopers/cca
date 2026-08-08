package web

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/config"
	"github.com/PaoDevelopers/cca/internal/db"
)

// Server carries the shared state of the web application.
type Server struct {
	config  config.Config
	pool    *pgxpool.Pool
	queries *db.Queries
	// The one function the token check needs, rather than the whole
	// JWKS client: nothing here manages key sets, and a narrower
	// field is one a test can supply.
	verifyKey jwt.Keyfunc
	wsHub     *WebSocketHub

	// The HMAC key behind the signed session cookies. Checked once
	// at startup so a missing or short key fails there rather than
	// on the first sign-in.
	sessionKey sessionKey

	// Fires at the next enrollment-window boundary so open pages
	// repaint; see window_timer.go for why it is allowed to miss.
	windowTimer *windowTimer
}
