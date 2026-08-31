package web

import (
	"net/http"

	"github.com/PaoDevelopers/cca/ui"
)

// The portal is React like the rest of the frontend, so there is
// nothing to render here. It is its own build under ui/portal, not a
// page of either panel: it is what somebody signed in to neither area,
// or to the other one, is shown first.
func (app *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleIndex")

	http.ServeFileFS(w, r, ui.Dist, "portal/dist/index.html")
}

// Who the caller is signed in as, in each area, empty where they are
// not. The portal's only data.
type sessionNames struct {
	Student string `json:"student"`
	Admin   string `json:"admin"`
}

// handleSession answers for a caller who may be nobody, so being signed
// in to neither area is an ordinary 200 with two empty strings rather
// than a 401: the portal asks precisely because it does not know, and
// its doors open either way. Both middlewares report a real outage
// properly on the pages that actually need a session.
//
// The session names the student but carries nothing else, so the portal
// names them by id rather than by name. Reading the roster for a
// decoration on a page that does not need a session is not worth the
// query.
func (app *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleSession")

	var names sessionNames

	if student, err := app.authenticateStudent(r); err == nil {
		names.Student = student.ID
	}

	if admin, err := app.authenticateAdmin(r); err == nil {
		names.Admin = admin.Username
	}

	app.writeJSON(r, w, names)
}
