package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPICourses(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(courses)
}
