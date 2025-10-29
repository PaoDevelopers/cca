package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPICategories(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPICategories")
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		app.apiError(r, w, http.StatusInternalServerError, err.Error())
		return
	}

	app.writeJSON(r, w, http.StatusOK, categories, slog.String("resource", "categories"))
}
