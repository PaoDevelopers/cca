package main

import (
	"net/http"
)

func (app *App) handleAdmStudents(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	app.admRenderTemplate(w, "students", "")
}
func (app *App) handleAdmStudentsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
func (app *App) handleAdmStudentsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
func (app *App) handleAdmStudentsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
}
