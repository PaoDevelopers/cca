package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserInfo interface {
	isUserInfo()
}

type UserInfoStudent db.GetStudentBySessionRow

func (u *UserInfoStudent) isUserInfo() {}

type UserInfoAdmin db.GetAdminBySessionRow

func (u *UserInfoAdmin) isUserInfo() {}

func (app *App) authenticateRequest(r *http.Request) (UserInfo, error) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil, fmt.Errorf("Failed fetching cookie: %w", err)
	}

	ty, st, ok := strings.Cut(cookie.Value, ":")
	if !ok {
		return nil, fmt.Errorf("Malformed session cookie contains no separator")
	}

	switch ty {
	case "student":
		u, err := app.queries.GetStudentBySession(
			r.Context(),
			pgtype.Text{
				String: st,
				Valid:  true,
			},
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, fmt.Errorf("Failed fetching student by session: %w", err)
		}
		uu := UserInfoStudent(u)
		return &uu, nil
	case "admin":
		u, err := app.queries.GetAdminBySession(
			r.Context(),
			pgtype.Text{
				String: st,
				Valid:  true,
			},
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, fmt.Errorf("Failed fetching admin by session: %w", err)
		}
		uu := UserInfoAdmin(u)
		return &uu, nil
	default:
		return nil, fmt.Errorf("Malformed session cookie contains unknown session type")
	}
}

func (app *App) studentOnly(handler func(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ui, err := app.authenticateRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized\n"+err.Error(), http.StatusUnauthorized)
			return
		}
		sui, ok := ui.(*UserInfoStudent)
		if !ok {
			http.Error(w, "Student-only endpoint", http.StatusForbidden)
			return
		}
		handler(w, r, sui)
	}
}

func (app *App) adminOnly(handler func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ui, err := app.authenticateRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized\n"+err.Error(), http.StatusUnauthorized)
			return
		}
		aui, ok := ui.(*UserInfoAdmin)
		if !ok {
			http.Error(w, "Admin-only endpoint", http.StatusForbidden)
			return
		}
		handler(w, r, aui)
	}
}
