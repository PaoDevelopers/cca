package main

import (
	"crypto/rand"
	"errors"
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\nMalformed form", http.StatusBadRequest)
		return
	}

	if e := r.PostFormValue("error"); e != "" {
		ed := r.PostFormValue("error_description")
		http.Error(w, "Bad Request\nExternal error\n"+e+"\n"+ed, http.StatusBadRequest)
		return
	}

	idts := r.PostFormValue("id_token")
	if idts == "" {
		http.Error(w, "Bad Request\nID token expected but not found", http.StatusBadRequest)
		return
	}

	idt, err := jwt.ParseWithClaims(idts, &Claims{}, app.kf.Keyfunc)
	if err != nil {
		http.Error(w, "Bad Request\nUnparsable JWT", http.StatusBadRequest)
		return
	}

	claims, ok := idt.Claims.(*Claims)

	switch {
	case !ok:
		http.Error(w, "Bad Request\nInvalid JWT claims", http.StatusBadRequest)
		return
	case idt.Valid:
		break
	case errors.Is(err, jwt.ErrTokenMalformed):
		http.Error(w, "Bad Request\nMalformed JWT", http.StatusBadRequest)
		return
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		http.Error(w, "Bad Request\nInvalid JWT signature", http.StatusBadRequest)
		return
	case errors.Is(err, jwt.ErrTokenExpired):
		http.Error(w, "Bad Request\nJWT expired", http.StatusBadRequest)
		return
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		http.Error(w, "Bad Request\nJWT not valid yet", http.StatusBadRequest)
		return
	default:
		http.Error(w, "Bad Request\nInvalid JWT", http.StatusBadRequest)
		return
	}

	claims.Email = strings.ToLower(claims.Email)
	lp, dp, ok := strings.Cut(claims.Email, "@")
	if !ok {
		http.Error(w, "Bad Request\nInvalid email address", http.StatusBadRequest)
		return
	}
	if dp != "ykpaoschool.cn" && dp != "stu.ykpaoschool.cn" {
		http.Error(w, "Unauthorized\nInvalid email address domain-part", http.StatusUnauthorized)
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
			http.Error(w, "Internal Server Error\nCannot set admin session token", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
	} else {
		sid, err := strconv.ParseInt(strings.TrimLeft(lp, "sS"), 10, 64)
		if err != nil {
			http.Error(w, "Unauthorized\nInvalid student ID", http.StatusUnauthorized)
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
				http.Error(w, "Unauthorized\nStudent ID not found in database", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Internal Server Error\nCannot set student session token", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/student/", http.StatusSeeOther)
	}
}
