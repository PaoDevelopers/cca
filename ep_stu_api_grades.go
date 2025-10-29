package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPIGrades(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIGrades", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	agrs, err := app.AbsGrades(r.Context())
	if err != nil {
		app.apiError(r, w, http.StatusInternalServerError, err.Error())
		return
	}

	app.writeJSON(r, w, http.StatusOK, agrs, slog.Int64("student_id", sui.ID))
}
