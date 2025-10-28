package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPICategories(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(categories)
}
