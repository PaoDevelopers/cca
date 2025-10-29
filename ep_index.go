package main

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
)

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleIndex")
	// TODO: Consider rendering a welcome and login page.
	target := fmt.Sprintf(
		"%s?client_id=%s&response_type=id_token%%20code&redirect_uri=%s%%2Fauth&response_mode=form_post&scope=openid+profile+email+User.Read&nonce=%s",
		app.config.OIDC.Authorize,
		app.config.OIDC.Client,
		app.config.URL,
		rand.Text(),
	)
	app.logInfo(r, "redirecting to oidc authorize", slog.String("target", target))
	http.Redirect(
		w,
		r,
		target,
		http.StatusSeeOther,
	)
}
