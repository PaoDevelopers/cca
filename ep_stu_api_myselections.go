package main

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleStuAPIMySelections(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	get := func() bool {
		selections, err := app.queries.GetSelectionsByStudent(r.Context(), sui.ID)
		if err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
			return true
		}
		json.NewEncoder(w).Encode(selections)
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
			apiError(w, http.StatusBadRequest, err)
			return
		}
		err = app.queries.DeleteChoiceByStudentAndCourse(r.Context(),
			db.DeleteChoiceByStudentAndCourseParams{
				PStudentID: sui.ID,
				PCourseID:  s,
			},
		)
		if err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if get() {
			return
		}
	case http.MethodPut:
		var s string
		err := json.NewDecoder(r.Body).Decode(&s)
		if err != nil {
			apiError(w, http.StatusBadRequest, err)
			return
		}
		err = app.queries.NewSelection(r.Context(), db.NewSelectionParams{
			StudentID:     sui.ID,
			CourseID:      s,
			SelectionType: "no",
		})
		if err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if get() {
			return
		}
	default:
		apiError(w, http.StatusMethodNotAllowed, nil)
	}
}
