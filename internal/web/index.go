package web

import "net/http"

func (app *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	app.logRequestStart(r, "handleIndex")

	// The landing page only decorates its links with a name, so a
	// lookup that fails is not worth failing the page over: it renders
	// as though nobody is signed in. Both middlewares report the
	// outage properly on the pages that actually need a session.
	var data indexData

	// The session names the student but carries nothing else, so the
	// landing page greets them by id rather than by name. Reading the
	// roster for a decoration on a page that does not need a session
	// is not worth the query.
	if student, err := app.authenticateStudent(r); err == nil {
		data.StudentName = student.ID
	}

	if admin, err := app.authenticateAdmin(r); err == nil {
		data.AdminName = admin.Username
	}

	app.renderPage(w, r, indexTemplate, data)
}
