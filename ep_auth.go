package main

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Claims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func (app *App) handleAuth(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleAuth")
	if r.Method != http.MethodPost {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil)
		return
	}

	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMalformed form", err)
		return
	}

	if e := r.PostFormValue("error"); e != "" {
		ed := r.PostFormValue("error_description")
		app.respondHTTPError(
			r,
			w,
			http.StatusBadRequest,
			"Bad Request\nExternal error\n"+e+"\n"+ed,
			errors.New("external auth error"),
			slog.String("external_error", e),
			slog.String("external_description", ed),
		)
		return
	}

	if app.config.OIDC.Bypass && r.PostFormValue("bypass") != "" {
		sid, err := strconv.ParseInt(strings.TrimLeft(r.PostFormValue("bypass"), "sS"), 10, 64)
		if err != nil {
			app.respondHTTPError(r, w, http.StatusUnauthorized, "Unauthorized\nInvalid student ID", nil)
		}
		st := rand.Text()
		tst := "student:" + st
		cookie := http.Cookie{
			Name:     "session",
			Value:    tst,
			SameSite: http.SameSiteLaxMode,
			HttpOnly: true,
			Secure:   true,
			Expires:  time.Now().Add(72 * time.Hour),
		}
		http.SetCookie(w, &cookie)
		_, err = app.queries.SetStudentSession(
			r.Context(),
			db.SetStudentSessionParams{
				SessionToken: pgtype.Text{String: st, Valid: true},
				ID:           sid,
			},
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				app.respondHTTPError(
					r,
					w,
					http.StatusUnauthorized,
					"Unauthorized\nStudent ID not found in database",
					err,
					slog.Int64("student_id", sid),
				)
				return
			}
			app.respondHTTPError(
				r,
				w,
				http.StatusInternalServerError,
				"Internal Server Error\nCannot set student session token",
				err,
				slog.Int64("student_id", sid),
			)
			return
		}

		app.logInfo(r, "BYPASS student authentication successful", slog.Int64("student_id", sid))
		http.Redirect(w, r, "/student/", http.StatusSeeOther)
	}

	idts := r.PostFormValue("id_token")
	if idts == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nID token expected but not found", nil)
		return
	}

	idt, err := jwt.ParseWithClaims(idts, &Claims{}, app.kf.Keyfunc)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnparsable JWT", err)
		return
	}

	claims, ok := idt.Claims.(*Claims)

	switch {
	case !ok:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid JWT claims", nil)
		return
	case idt.Valid:
		break
	case errors.Is(err, jwt.ErrTokenMalformed):
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMalformed JWT", err)
		return
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid JWT signature", err)
		return
	case errors.Is(err, jwt.ErrTokenExpired):
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nJWT expired", err)
		return
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nJWT not valid yet", err)
		return
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid JWT", err)
		return
	}

	claims.Email = strings.ToLower(claims.Email)
	lp, dp, ok := strings.Cut(claims.Email, "@")
	if !ok {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid email address", nil)
		return
	}
	if dp != "ykpaoschool.cn" && dp != "stu.ykpaoschool.cn" {
		app.respondHTTPError(
			r,
			w,
			http.StatusUnauthorized,
			"Unauthorized\nInvalid email address domain-part",
			nil,
			slog.String("domain", dp),
			slog.String("email", claims.Email),
		)
		return
	}

	_, isAdmin := app.config.Admins[lp]

	st := rand.Text()
	tst := ""
	if isAdmin {
		tst = "admin:" + st
	} else {
		tst = "student:" + st
	}

	cookie := http.Cookie{
		Name:     "session",
		Value:    tst,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   true,
		Expires:  time.Now().Add(72 * time.Hour),
	}
	http.SetCookie(w, &cookie)

	if isAdmin {
		err = app.queries.SetAdminSession(
			r.Context(),
			db.SetAdminSessionParams{
				SessionToken: pgtype.Text{String: st, Valid: true},
				Username:     lp,
			},
		)
		if err != nil {
			app.respondHTTPError(
				r,
				w,
				http.StatusInternalServerError,
				"Internal Server Error\nCannot set admin session token",
				err,
				slog.String("admin_username", lp),
			)
			return
		}

		app.logInfo(r, "admin authentication successful", slog.String("admin_username", lp))
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
	} else {
		sid, err := strconv.ParseInt(strings.TrimLeft(lp, "sS"), 10, 64)
		if err != nil {
			app.respondHTTPError(
				r,
				w,
				http.StatusUnauthorized,
				"Unauthorized\nInvalid student ID",
				err,
				slog.String("label", lp),
			)
			return
		}
		_, err = app.queries.SetStudentSession(
			r.Context(),
			db.SetStudentSessionParams{
				SessionToken: pgtype.Text{String: st, Valid: true},
				ID:           sid,
			},
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				app.respondHTTPError(
					r,
					w,
					http.StatusUnauthorized,
					"Unauthorized\nStudent ID not found in database",
					err,
					slog.Int64("student_id", sid),
				)
				return
			}
			app.respondHTTPError(
				r,
				w,
				http.StatusInternalServerError,
				"Internal Server Error\nCannot set student session token",
				err,
				slog.Int64("student_id", sid),
			)
			return
		}

		app.logInfo(r, "student authentication successful", slog.Int64("student_id", sid))
		http.Redirect(w, r, "/student/", http.StatusSeeOther)
	}
}
