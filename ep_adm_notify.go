package main

import (
	"net/http"
)

func (app *App) handleAdmNotify(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method == http.MethodGet {
		app.admRenderTemplate(w, "notify", nil)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	app.broker.Broadcast(BrokerMsg{event: "notify", data: r.FormValue("text")})

	http.Redirect(w, r, "/admin/notify", http.StatusSeeOther)
}
