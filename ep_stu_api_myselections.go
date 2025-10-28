package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPIMySelections(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	get := func() {
		selections, err := app.queries.GetSelectionsByStudent(r.Context(), sui.ID)
		if err != nil {
			apiError(w, http.StatusInternalServerError, err.Error())
		}
		json.NewEncoder(w).Encode(selections)
	}

	switch r.Method {
	case http.MethodGet:
		get()
	case http.MethodDelete:
		get()
	case http.MethodPut:
		get()
	default:
	}
}
