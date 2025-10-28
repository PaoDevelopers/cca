package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPIGrades(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	agrs, err := app.AbsGrades(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(agrs)
}
