package web

import (
	"log/slog"
	"net/http"
)

// Periods carry a display name now, so the whole row goes out rather
// than a list of bare ids: the timetable labels its rows with the name
// and keys them by the id.
func (app *Server) handleStuAPIPeriods(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIPeriods")

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	periods, err := app.queries.GetPeriods(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, periods, slog.String("resource", "periods"))
}
