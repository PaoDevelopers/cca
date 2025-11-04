// Package main implements the ticketing web server.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const userContextKey contextKey = "session-user"

type SessionUser struct {
	ID       int64
	Username string
	IsAdmin  bool
}

func UserFromContext(ctx context.Context) (*SessionUser, bool) {
	val := ctx.Value(userContextKey)
	if val == nil {
		return nil, false
	}
	user, ok := val.(*SessionUser)
	return user, ok
}

type SessionManager struct {
	Store           *Store
	Logger          *slog.Logger
	CookieName      string
	SessionDuration time.Duration
}

func (m *SessionManager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(m.CookieName)
		if err != nil {
			if !errors.Is(err, http.ErrNoCookie) {
				m.Logger.WarnContext(r.Context(), "failed to read session cookie", "error", err)
			}
			next.ServeHTTP(w, r)
			return
		}

		now := time.Now()
		session, err := m.Store.GetSessionWithUser(r.Context(), cookie.Value, now)
		if err != nil {
			m.Logger.ErrorContext(r.Context(), "failed to fetch session", "error", err)
			clearCookie(w, m.CookieName)
			next.ServeHTTP(w, r)
			return
		}
		if session == nil {
			clearCookie(w, m.CookieName)
			next.ServeHTTP(w, r)
			return
		}

		user := &SessionUser{
			ID:       session.User.ID,
			Username: session.User.Username,
			IsAdmin:  session.User.IsAdmin,
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		newExpiry := now.Add(m.SessionDuration)
		if err := m.Store.RenewSession(ctx, session.Token, newExpiry); err != nil {
			m.Logger.ErrorContext(ctx, "failed to renew session", "error", err)
			clearCookie(w, m.CookieName)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		setCookie(w, m.CookieName, session.Token, newExpiry)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *SessionManager) CreateSession(ctx context.Context, w http.ResponseWriter, user *User) error {
	token := rand.Text()
	expires := time.Now().Add(m.SessionDuration)
	if err := m.Store.CreateSession(ctx, token, user.ID, expires); err != nil {
		return err
	}
	setCookie(w, m.CookieName, token, expires)
	return nil
}

func (m *SessionManager) DestroySession(ctx context.Context, w http.ResponseWriter, token string) error {
	if token != "" {
		if err := m.Store.DeleteSession(ctx, token); err != nil {
			return err
		}
	}
	clearCookie(w, m.CookieName)
	return nil
}

func setCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
