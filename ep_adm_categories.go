package main

import (
	"net/http"
)

func (app *App) handleAdmCategories(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "categories", categories)
}

func (app *App) handleAdmCategoriesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to add an empty category ID, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.NewCategory(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.broker.Broadcast(BrokerMsg{event: "invalidate_categories"})

	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}
func (app *App) handleAdmCategoriesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to delete an empty category ID, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.DeleteCategory(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.broker.Broadcast(BrokerMsg{event: "invalidate_categories"})

	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}
