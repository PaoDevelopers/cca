package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleAdmGrades(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGrades", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	grades2, err := app.AbsGrades(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "grades", struct {
		Grades     []AbsGradesRow
		Categories []string
	}{
		grades2,
		categories,
	}, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmGradesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesNew", slog.String("admin_username", aui.Username))
	grade := r.FormValue("grade")
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add an empty grade name, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.NewGrade(r.Context(), grade)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("grade", grade))
		return
	}

	app.logInfo(r, "created grade", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after new grade", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesBulkEnabledUpdate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesBulkEnabledUpdate", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	var grades []string
	for _, grade := range r.PostForm {
		if len(grade) != 1 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nDuplicate or zero-length value sets in your form...?", nil, slog.String("admin_username", aui.Username))
			return
		}
		grades = append(grades, grade[0])
	}

	err = app.queries.SetGradesBulkEnabledUpdate(r.Context(), grades)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	app.logInfo(r, "updated grades enabled flags", slog.String("admin_username", aui.Username))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after bulk update", slog.String("admin_username", aui.Username))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}

// Is this even still used?
func (app *App) handleAdmGradesEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesEdit", slog.String("admin_username", aui.Username))
	grade := r.FormValue("grade")
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit an empty grade name, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	enabled := r.FormValue("enabled")

	err := app.queries.SetGradeEnabled(r.Context(), db.SetGradeEnabledParams{
		Enabled: enabled != "",
		Grade:   grade,
	})
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("grade", grade))
		return
	}

	app.logInfo(r, "edited grade flag", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after edit grade", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesDelete", slog.String("admin_username", aui.Username))
	grade := r.FormValue("grade")
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete an empty grade name, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.DeleteGrade(r.Context(), grade)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("grade", grade))
		return
	}

	app.logInfo(r, "deleted grade", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after delete grade", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}

func (app *App) handleAdmGradesNewRequirementGroup(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesNewRequirementGroup", slog.String("admin_username", aui.Username))
	grade := r.FormValue("grade")
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a requirement group for an empty grade name, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}
	minCountString := r.FormValue("min_count")
	minCount, err := strconv.ParseInt(minCountString, 10, 32)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a requirement group with a non-integer min count. That is not allowed.", err, slog.String("admin_username", aui.Username))
		return
	}
	var categories []string
	for key, value := range r.PostForm {
		if !strings.HasPrefix(key, "category-") {
			continue
		}
		if len(value) != 1 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nDuplicate or zero-length value sets in your form...?", nil, slog.String("admin_username", aui.Username))
			return
		}
		categories = append(categories, value[0])
	}

	err = app.queries.NewRequirementGroup(r.Context(), db.NewRequirementGroupParams{
		Grade:    grade,
		MinCount: minCount,
		Column3:  categories,
	})
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("grade", grade))
		return
	}

	app.logInfo(r, "created grade requirement group", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after new requirement group", slog.String("admin_username", aui.Username), slog.String("grade", grade))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesDeleteRequirementGroup(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmGradesDeleteRequirementGroup", slog.String("admin_username", aui.Username))
	idString := r.FormValue("id")
	id, err := strconv.ParseInt(idString, 10, 32)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a requirement group with an ID that doesn't seem to be valid", err, slog.String("admin_username", aui.Username))
		return
	}
	err = app.queries.DeleteRequirementGroup(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("requirement_group_id", id))
		return
	}

	app.logInfo(r, "deleted grade requirement group", slog.String("admin_username", aui.Username), slog.Int64("requirement_group_id", id))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_grades"})

	app.logInfo(r, "redirecting after delete requirement group", slog.String("admin_username", aui.Username), slog.Int64("requirement_group_id", id))
	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
