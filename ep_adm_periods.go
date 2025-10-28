package main

import (
	"net/http"
)

func (app *App) handleAdmPeriods(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "periods", periods)
}

func (app *App) handleAdmPeriodsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to add an empty period ID, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.NewPeriod(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.broker.Broadcast(BrokerMsg{event: "invalidate_periods"})

	http.Redirect(w, r, "/admin/periods", http.StatusSeeOther)
}
func (app *App) handleAdmPeriodsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to delete an empty period ID, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.DeletePeriod(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.broker.Broadcast(BrokerMsg{event: "invalidate_periods"})

	http.Redirect(w, r, "/admin/periods", http.StatusSeeOther)
}
