package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPIMySelections(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	json.NewEncoder(w).Encode(sui)
}
