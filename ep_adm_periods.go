package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleAdmPeriods(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmPeriods", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "periods", periods, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmPeriodsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmPeriodsNew", slog.String("admin_username", aui.Username))
	id := r.FormValue("id")
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add an empty period ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.NewPeriod(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("period_id", id))
		return
	}

	app.logInfo(r, logMsgAdminPeriodsCreate, slog.String("admin_username", aui.Username), slog.String("period_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_periods"))

	app.logInfo(r, logMsgAdminPeriodsCreateRedirect, slog.String("admin_username", aui.Username), slog.String("period_id", id))
	http.Redirect(w, r, "/admin/periods", http.StatusSeeOther)
}

func (app *App) handleAdmPeriodsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmPeriodsDelete", slog.String("admin_username", aui.Username))
	id := r.FormValue("id")
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete an empty period ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.DeletePeriod(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("period_id", id))
		return
	}

	app.logInfo(r, logMsgAdminPeriodsDelete, slog.String("admin_username", aui.Username), slog.String("period_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_periods"))

	app.logInfo(r, logMsgAdminPeriodsDeleteRedirect, slog.String("admin_username", aui.Username), slog.String("period_id", id))
	http.Redirect(w, r, "/admin/periods", http.StatusSeeOther)
}
