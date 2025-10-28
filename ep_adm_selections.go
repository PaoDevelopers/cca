package main

import (
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

// TODO: See how SSEs should be handled here. We may need a way to map from usernames to connections.

func (app *App) handleAdmSelections(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	selections, err := app.queries.GetSelections(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	students, err := app.queries.GetStudents(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "selections", struct {
		Selections     []db.GetSelectionsRow
		Students       []db.Student
		Courses        []db.Course
		SelectionTypes []db.SelectionType
	}{
		Selections:     selections,
		Students:       students,
		Courses:        courses,
		SelectionTypes: []db.SelectionType{db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce},
	})
}

func (app *App) handleAdmSelectionsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		http.Error(w, "Bad Request\nYou are trying to add a selection without a student ID, which is not allowed", http.StatusBadRequest)
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	courseID := strings.TrimSpace(r.FormValue("course_id"))
	if courseID == "" {
		http.Error(w, "Bad Request\nYou are trying to add a selection without a course ID, which is not allowed", http.StatusBadRequest)
		return
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		http.Error(w, "Bad Request\nUnknown selection type", http.StatusBadRequest)
		return
	}

	err = app.queries.NewSelection(r.Context(), db.NewSelectionParams{
		StudentID:     studentID,
		CourseID:      courseID,
		SelectionType: selectionType,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a selection without a student ID, which is not allowed", http.StatusBadRequest)
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a selection without a period, which is not allowed", http.StatusBadRequest)
		return
	}

	courseID := strings.TrimSpace(r.FormValue("course_id"))
	if courseID == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a selection without a course ID, which is not allowed", http.StatusBadRequest)
		return
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		http.Error(w, "Bad Request\nUnknown selection type", http.StatusBadRequest)
		return
	}

	err = app.queries.UpdateSelection(r.Context(), db.UpdateSelectionParams{
		StudentID:     studentID,
		CourseID:      courseID,
		Period:        period,
		SelectionType: selectionType,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		http.Error(w, "Bad Request\nYou are trying to delete a selection without a student ID, which is not allowed", http.StatusBadRequest)
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		http.Error(w, "Bad Request\nYou are trying to delete a selection without a period, which is not allowed", http.StatusBadRequest)
		return
	}

	err = app.queries.DeleteSelection(r.Context(), db.DeleteSelectionParams{
		StudentID: studentID,
		Period:    period,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}
