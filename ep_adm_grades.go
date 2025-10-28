package main

import (
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleAdmGrades(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	grades2, err := app.AbsGrades(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "grades", struct {
		Grades     []AbsGradesRow
		Categories []string
	}{
		grades2,
		categories,
	})
}

func (app *App) handleAdmGradesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	grade := r.FormValue("grade")
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to add an empty grade name, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.NewGrade(r.Context(), grade)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesBulkEnabledUpdate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	var grades []string
	for _, grade := range r.PostForm {
		if len(grade) != 1 {
			http.Error(w, "Bad Request\nDuplicate or zero-length value sets in your form...?", http.StatusBadRequest)
			return
		}
		grades = append(grades, grade[0])
	}

	err = app.queries.SetGradesBulkEnabledUpdate(r.Context(), grades)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	grade := r.FormValue("grade")
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to edit an empty grade name, which is not allowed", http.StatusBadRequest)
		return
	}

	enabled := r.FormValue("enabled")

	err := app.queries.SetGradeEnabled(r.Context(), db.SetGradeEnabledParams{
		Enabled: enabled != "",
		Grade:   grade,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	grade := r.FormValue("grade")
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to delete an empty grade name, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.DeleteGrade(r.Context(), grade)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}

func (app *App) handleAdmGradesNewRequirementGroup(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	grade := r.FormValue("grade")
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to add a requirement group for an empty grade name, which is not allowed", http.StatusBadRequest)
		return
	}
	minCountString := r.FormValue("min_count")
	minCount, err := strconv.ParseInt(minCountString, 10, 32)
	if err != nil {
		http.Error(w, "Bad Request\nYou are trying to add a requirement group with a non-integer min count. That is not allowed.", http.StatusBadRequest)
		return
	}
	var categories []string
	for key, value := range r.PostForm {
		if !strings.HasPrefix(key, "category-") {
			continue
		}
		if len(value) != 1 {
			http.Error(w, "Bad Request\nDuplicate or zero-length value sets in your form...?", http.StatusBadRequest)
		}
		categories = append(categories, value[0])
	}

	err = app.queries.NewRequirementGroup(r.Context(), db.NewRequirementGroupParams{
		Grade:    grade,
		MinCount: minCount,
		Column3:  categories,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
func (app *App) handleAdmGradesDeleteRequirementGroup(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	idString := r.FormValue("id")
	id, err := strconv.ParseInt(idString, 10, 32)
	if err != nil {
		http.Error(w, "Bad Request\nYou are trying to add a requirement group with an ID that doesn't seem to be valid", http.StatusBadRequest)
		return
	}
	err = app.queries.DeleteRequirementGroup(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/grades", http.StatusSeeOther)
}
