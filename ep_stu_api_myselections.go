package main

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleStuAPIMySelections(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIMySelections", slog.Int64("student_id", sui.ID))
	get := func() bool {
		selections, err := app.queries.GetSelectionsByStudent(r.Context(), sui.ID)
		if err != nil {
			app.apiError(r, w, http.StatusInternalServerError, err.Error(), slog.Int64("student_id", sui.ID))
			return true
		}
		app.writeJSON(r, w, http.StatusOK, selections, slog.Int64("student_id", sui.ID))
		return false
	}

	switch r.Method {
	case http.MethodGet:
		if get() {
			return
		}
	case http.MethodDelete:
		var s string
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			app.apiError(r, w, http.StatusBadRequest, err, slog.String("operation", "delete_selection"), slog.Int64("student_id", sui.ID))
			return
		}
		err = app.queries.DeleteChoiceByStudentAndCourse(r.Context(),
			db.DeleteChoiceByStudentAndCourseParams{
				PStudentID: sui.ID,
				PCourseID:  s,
			},
		)
		if err != nil {
			app.apiError(r, w, http.StatusInternalServerError, err.Error(), slog.String("operation", "delete_selection"), slog.Int64("student_id", sui.ID), slog.String("course_id", s))
			return
		}
		app.logInfo(r, "deleted selection", slog.Int64("student_id", sui.ID), slog.String("course_id", s))
		app.broadcastCourseCounts(r, []string{s})
		if get() {
			return
		}
	case http.MethodPut:
		var s string
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			app.apiError(r, w, http.StatusBadRequest, err, slog.String("operation", "new_selection"), slog.Int64("student_id", sui.ID))
			return
		}
		err = app.queries.NewSelection(r.Context(), db.NewSelectionParams{
			StudentID:     sui.ID,
			CourseID:      s,
			SelectionType: "normal",
		})
		if err != nil {
			app.apiError(r, w, http.StatusInternalServerError, err.Error(), slog.String("operation", "new_selection"), slog.Int64("student_id", sui.ID), slog.String("course_id", s))
			return
		}
		app.logInfo(r, "created selection", slog.Int64("student_id", sui.ID), slog.String("course_id", s))
		app.broadcastCourseCounts(r, []string{s})
		if get() {
			return
		}
	default:
		app.apiError(r, w, http.StatusMethodNotAllowed, nil)
	}
}
