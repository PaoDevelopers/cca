package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
)

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	// TODO: Consider rendering a welcome and login page.
	http.Redirect(
		w,
		r,
		fmt.Sprintf(
			"%s?client_id=%s&response_type=id_token%%20code&redirect_uri=%s%%2Fauth&response_mode=form_post&scope=openid+profile+email+User.Read&nonce=%s",
			app.config.OIDC.Authorize,
			app.config.OIDC.Client,
			app.config.URL,
			rand.Text(),
		),
		http.StatusSeeOther,
	)
}
