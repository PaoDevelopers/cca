package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

// TODO: See how SSEs should be handled here. We may need a way to map from usernames to connections.

func (app *App) handleAdmSelections(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelections", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	selections, err := app.queries.GetSelections(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	students, err := app.queries.GetStudents(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "selections", struct {
		Selections     []db.GetSelectionsRow
		Students       []db.Student
		Courses        []db.Course
		SelectionTypes []db.SelectionType
	}{
		Selections:     selections,
		Students:       students,
		Courses:        courses,
		SelectionTypes: []db.SelectionType{db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce},
	}, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmSelectionsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsNew", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	rawStudentIDs := r.PostForm["student_ids"]
	if len(rawStudentIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nSelect at least one student", nil, slog.String("admin_username", aui.Username))
		return
	}

	var studentIDs []int64
	studentSeen := make(map[int64]struct{}, len(rawStudentIDs))
	for _, raw := range rawStudentIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", parseErr, slog.String("admin_username", aui.Username))
			return
		}
		if _, ok := studentSeen[id]; ok {
			continue
		}
		studentSeen[id] = struct{}{}
		studentIDs = append(studentIDs, id)
	}
	if len(studentIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nNo valid student IDs provided", nil, slog.String("admin_username", aui.Username))
		return
	}

	rawCourseIDs := r.PostForm["course_ids"]
	if len(rawCourseIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nSelect at least one course", nil, slog.String("admin_username", aui.Username))
		return
	}

	var courseIDs []string
	courseSeen := make(map[string]struct{}, len(rawCourseIDs))
	for _, raw := range rawCourseIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := courseSeen[id]; ok {
			continue
		}
		courseSeen[id] = struct{}{}
		courseIDs = append(courseIDs, id)
	}
	if len(courseIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nNo valid course IDs provided", nil, slog.String("admin_username", aui.Username))
		return
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown selection type", nil, slog.String("admin_username", aui.Username))
		return
	}

	if err = app.queries.NewSelectionsBulk(r.Context(), db.NewSelectionsBulkParams{
		Column1:       studentIDs,
		Column2:       courseIDs,
		SelectionType: selectionType,
	}); err != nil {
		app.respondHTTPError(
			r,
			w,
			http.StatusInternalServerError,
			"Internal Server Error\n"+err.Error(),
			err,
			slog.String("admin_username", aui.Username),
			slog.Any("student_ids", studentIDs),
			slog.Any("course_ids", courseIDs),
		)
		return
	}

	app.logInfo(
		r,
		"created selections",
		slog.String("admin_username", aui.Username),
		slog.Any("student_ids", studentIDs),
		slog.Any("course_ids", courseIDs),
		slog.String("selection_type", string(selectionType)),
	)
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsEdit", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without a student ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without a period, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}

	courseID := strings.TrimSpace(r.FormValue("course_id"))
	if courseID == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without a course ID, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNo, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown selection type", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID))
		return
	}

	if err = app.queries.UpdateSelection(r.Context(), db.UpdateSelectionParams{
		StudentID:     studentID,
		CourseID:      courseID,
		Period:        period,
		SelectionType: selectionType,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID), slog.String("period", period))
		return
	}

	app.logInfo(r, "updated selection", slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID), slog.String("period", period), slog.String("selection_type", string(selectionType)))
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsDelete", slog.String("admin_username", aui.Username))
	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a selection without a student ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a selection without a period, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}

	if err = app.queries.DeleteSelection(r.Context(), db.DeleteSelectionParams{
		StudentID: studentID,
		Period:    period,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("period", period))
		return
	}

	app.logInfo(r, "deleted selection", slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("period", period))
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}
