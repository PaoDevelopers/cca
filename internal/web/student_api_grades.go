package web

import (
	"log/slog"
	"net/http"
)

func (app *Server) handleStuAPIGrades(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIGrades", slog.String("student_id", sui.ID))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	rows, err := app.Grades(ctx)
	if err != nil {
		app.apiInternalError(r, w, err)

		return
	}

	app.writeJSON(r, w, rows, slog.String("student_id", sui.ID))
}
