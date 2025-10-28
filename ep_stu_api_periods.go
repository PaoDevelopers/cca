package main

import (
	"encoding/json"
	"net/http"
)

func (app *App) handleStuAPIPeriods(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(periods)
}
