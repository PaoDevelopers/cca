package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleAdmCategories(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCategories", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "categories", categories, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmCategoriesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCategoriesNew", slog.String("admin_username", aui.Username))
	id := r.FormValue("id")
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add an empty category ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.NewCategory(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("category_id", id))
		return
	}

	app.logInfo(r, logMsgAdminCategoriesCreate, slog.String("admin_username", aui.Username), slog.String("category_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_categories"))

	app.logInfo(r, logMsgAdminCategoriesCreateRedirect, slog.String("admin_username", aui.Username), slog.String("category_id", id))
	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}

func (app *App) handleAdmCategoriesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCategoriesDelete", slog.String("admin_username", aui.Username))
	id := r.FormValue("id")
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete an empty category ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.DeleteCategory(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("category_id", id))
		return
	}

	app.logInfo(r, logMsgAdminCategoriesDelete, slog.String("admin_username", aui.Username), slog.String("category_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_categories"))

	app.logInfo(r, logMsgAdminCategoriesDeleteRedirect, slog.String("admin_username", aui.Username), slog.String("category_id", id))
	http.Redirect(w, r, "/admin/categories", http.StatusSeeOther)
}
