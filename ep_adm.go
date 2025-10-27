package main

import (
	"net/http"
)

func (app *App) handleAdm(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	app.admRenderTemplate(w, "index", "")
}
