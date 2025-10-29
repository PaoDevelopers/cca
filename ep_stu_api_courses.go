package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleStuAPICourses(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPICourses", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		app.apiError(r, w, http.StatusInternalServerError, err.Error())
		return
	}

	app.writeJSON(r, w, http.StatusOK, courses, slog.Int64("student_id", sui.ID))
}
