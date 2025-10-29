package main

import (
	"errors"
	"fmt"
	"log/slog"
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
		return nil, fmt.Errorf("fetch cookie: %w", err)
	}

	ty, st, ok := strings.Cut(cookie.Value, ":")
	if !ok {
		return nil, fmt.Errorf("malformed session cookie contains no separator")
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
			return nil, fmt.Errorf("fetch student by session: %w", err)
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
			return nil, fmt.Errorf("fetch fetching admin by session: %w", err)
		}
		uu := UserInfoAdmin(u)
		return &uu, nil
	default:
		return nil, fmt.Errorf("malformed session cookie contains unknown session type")
	}
}

func (app *App) studentOnly(handlerName string, handler func(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, handlerName, slog.String("middleware", "studentOnly"))
		ui, err := app.authenticateRequest(r)
		if err != nil {
			app.respondHTTPError(
				r,
				w,
				http.StatusUnauthorized,
				"Unauthorized\nGo to the root URL (remove everything after the \"/\") to authenticate?\n"+err.Error(),
				err,
				slog.String("middleware", "studentOnly"),
			)
			return
		}
		sui, ok := ui.(*UserInfoStudent)
		if !ok {
			app.respondHTTPError(
				r,
				w,
				http.StatusForbidden,
				"Forbidden\nStudent-only endpoint\nGo to the root URL (remove everything after the \"/\") to authenticate?",
				nil,
				slog.String("middleware", "studentOnly"),
			)
			return
		}
		app.logInfo(r, "authenticated student request", slog.String("middleware", "studentOnly"), slog.Int64("student_id", sui.ID))
		handler(w, r, sui)
	}
}

func (app *App) adminOnly(handlerName string, handler func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		app.logRequestStart(r, handlerName, slog.String("middleware", "adminOnly"))
		ui, err := app.authenticateRequest(r)
		if err != nil {
			app.respondHTTPError(
				r,
				w,
				http.StatusUnauthorized,
				"Unauthorized\nGo to the root URL (remove everything after the \"/\") to authenticate?\n"+err.Error(),
				err,
				slog.String("middleware", "adminOnly"),
			)
			return
		}
		aui, ok := ui.(*UserInfoAdmin)
		if !ok {
			app.respondHTTPError(
				r,
				w,
				http.StatusForbidden,
				"Forbidden\nAdmin-only endpoint\nGo to the root URL (remove everything after the \"/\") to authenticate?",
				nil,
				slog.String("middleware", "adminOnly"),
			)
			return
		}
		app.logInfo(r, "authenticated admin request", slog.String("middleware", "adminOnly"), slog.String("admin_username", aui.Username))
		handler(w, r, aui)
	}
}
