package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleAdmPeriods(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmPeriods", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "periods", periods, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}
