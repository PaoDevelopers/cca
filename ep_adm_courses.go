package main

import (
	"net/http"
)

func (app *App) handleAdmCourses(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	app.admRenderTemplate(w, "courses", "")
}

func (app *App) handleAdmCoursesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
func (app *App) handleAdmCoursesEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
func (app *App) handleAdmCoursesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
