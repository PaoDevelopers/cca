package web

import (
	"log/slog"
	"net/http"
)

func (app *Server) handleStuAPICategories(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPICategories")

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	categories, err := app.queries.GetCategories(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, categories, slog.String("resource", "categories"))
}
