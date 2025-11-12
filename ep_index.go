package main

import (
	"crypto/rand"
	"log/slog"
	"net/http"
	"net/url"
)

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleIndex")
	// TODO: Consider rendering a welcome and login page.
	redirectURI := requestAbsoluteURL(r, "/auth")

	target, err := buildOIDCAuthorizeURL(app.config.OIDC.Authorize, app.config.OIDC.Client, redirectURI)
	if err != nil {
		app.respondHTTPError(
			r,
			w,
			http.StatusInternalServerError,
			"Internal Server Error\nCannot build OIDC authorize URL",
			err,
		)
		return
	}

	app.logInfo(r, logMsgAuthOIDCRedirect, slog.String("target", target))
	http.Redirect(
		w,
		r,
		target,
		http.StatusSeeOther,
	)
}

func buildOIDCAuthorizeURL(authorizeEndpoint, clientID, redirectURI string) (string, error) {
	u, err := url.Parse(authorizeEndpoint)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("response_type", "id_token code")
	q.Set("redirect_uri", redirectURI)
	q.Set("response_mode", "form_post")
	q.Set("scope", "openid profile email User.Read")
	q.Set("nonce", rand.Text())

	u.RawQuery = q.Encode()
	return u.String(), nil
}
