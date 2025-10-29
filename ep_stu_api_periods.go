package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPIPeriods(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIPeriods")
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.apiError(r, w, http.StatusInternalServerError, err.Error())
		return
	}

	app.writeJSON(r, w, http.StatusOK, periods, slog.String("resource", "periods"))
}
