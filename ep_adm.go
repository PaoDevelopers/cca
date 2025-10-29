package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleAdm(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdm", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "index", "", slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}
