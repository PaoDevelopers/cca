package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// A period is an id and a display name, like a category, so its
// create, rename and delete are the shared vocabulary handlers. What
// is its own is the order: periods have one and categories do not.
func (app *Server) periods() namedEntity {
	return namedEntity{
		noun:     "period",
		idField:  "period_id",
		resource: "periods",
		example:  `expected {"id": "MON1", "name": "Monday 16:00-17:00"}`,

		create: func(ctx context.Context, id, name string) error {
			return app.queries.NewPeriod(ctx, db.NewPeriodParams{ID: id, Name: name})
		},
		rename: func(ctx context.Context, id, name string) (int64, error) {
			return app.queries.RenamePeriod(ctx, db.RenamePeriodParams{ID: id, Name: name})
		},
		remove: func(ctx context.Context, id string) (int64, error) {
			return app.queries.DeletePeriod(ctx, id)
		},
	}
}

func (app *Server) apiPeriodsList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	periods, err := app.queries.GetPeriods(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, periods)
}

// Ordering is declarative: the request names the whole order, which is
// what the administrator is looking at when they drag a row. It is
// therefore idempotent, a double-submit is harmless, and there is no
// relative "move up" that reorders twice. set_period_order refuses an
// incomplete list, so a stale tab cannot silently drop a period that
// someone else added.
func (app *Server) apiPeriodsOrder(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	body, err := decodeBody[struct {
		IDs []string `json:"ids"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"ids": ["MON1", "TUE1"]}`, err)

		return
	}

	if err := app.queries.SetPeriodOrder(ctx, body.IDs); err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.logInfo(r, logMsgAdminPeriodsOrder, slog.String("admin_username", aui.Username), slog.Int("period_count", len(body.IDs)))
	app.wsHub.Broadcast(WSMessage("invalidate_periods"))
	// Course period lists are ordered by period sort_order.
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	w.WriteHeader(http.StatusNoContent)
}
