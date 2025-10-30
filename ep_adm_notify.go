package main

import (
	"log/slog"
	"net/http"
)

func (app *App) handleAdmNotify(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmNotify", slog.String("admin_username", aui.Username))
	if r.Method == http.MethodGet {
		if err := app.admRenderTemplate(w, r, "notify", nil, slog.String("admin_username", aui.Username)); err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
		}
		return
	}

	if r.Method != http.MethodPost {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	message := r.FormValue("text")
	app.logInfo(r, "broadcast admin notification", slog.String("admin_username", aui.Username))
	app.wsHub.Broadcast(WSMessage("notify," + message))

	app.logInfo(r, "redirecting after broadcast", slog.String("admin_username", aui.Username))
	http.Redirect(w, r, "/admin/notify", http.StatusSeeOther)
}
