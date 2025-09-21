package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/PaoDevelopers/cca/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Claims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func (app *App) setupJWKS() error {
	var err error
	app.kf, err = keyfunc.NewDefault([]string{app.config.OIDC.JWKS})
	return err
}

func (app *App) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	ui, err := app.authenticateRequest(r)
	if err != nil {
		http.Error(w, "Error authenticating request: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch ui := ui.(type) {
	case nil:
		json.NewEncoder(w).Encode(
			struct {
				Type      string `json:"type"`
				Authorize string `json:"authorize"`
			}{
				Type: "none",
				Authorize: fmt.Sprintf(
					"%s?client_id=%s&response_type=id_token%%20code&redirect_uri=%s%%2Fauth&response_mode=form_post&scope=openid+profile+email+User.Read&nonce=%s",
					app.config.OIDC.Authorize,
					app.config.OIDC.Client,
					app.config.URL,
					rand.Text(),
				),
			},
		)
	case *UserInfoStudent:
		json.NewEncoder(w).Encode(
			struct {
				Type     string `json:"type"`
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Grade    string `json:"grade"`
				LegalSex string `json:"legal_sex"`
			}{
				Type:     "student",
				ID:       ui.ID,
				Name:     ui.Name,
				Grade:    ui.Grade,
				LegalSex: string(ui.LegalSex),
			},
		)
	case *UserInfoAdmin:
		json.NewEncoder(w).Encode(
			struct {
				Type     string `json:"type"`
				ID       int64  `json:"id"`
				Username string `json:"username"`
			}{
				Type:     "student",
				ID:       ui.ID,
				Username: ui.Username,
			},
		)
	}
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
		Secure:   false, // TODO
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
	} else {
		sid, err := strconv.ParseInt(strings.TrimLeft(lp, "sS"), 10, 64)
		if err != nil {
			http.Error(w, "Unauthorized\nInvalid student ID", http.StatusUnauthorized)
			return
		}
		err = app.queries.SetStudentSession(
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
	}
}
