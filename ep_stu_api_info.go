package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPIInfo(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIInfo", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	app.writeJSON(r, w, http.StatusOK, sui, slog.Int64("student_id", sui.ID))
}
