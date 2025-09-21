package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/PaoDevelopers/cca/db"
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
		return nil, nil
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
