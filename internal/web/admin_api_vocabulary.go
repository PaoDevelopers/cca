package web

import (
	"context"
	"log/slog"
	"net/http"
)

// Categories and periods are the same thing twice.
//
// Each is an id and a display name, with no rules attached: creating
// one is a single insert, renaming one is a single update, deleting
// one is a single delete that the foreign keys refuse while anything
// still refers to it. Nothing about either goes through a write
// function, because there is nothing to judge.
//
// So the two sets of handlers are written once, parameterized by what
// differs — which table the statement names, what the log calls it,
// and which resource the browsers should re-read. Writing them twice
// meant two places to fix whenever the shape moved, which is how the
// write context came to be added to one and not the other.
//
// Grades are deliberately not folded in here: they carry a window, a
// budget cap, a category minimum and a requirement set, and only one
// of those is a plain statement.

// namedEntity is the vocabulary of one such table.
type namedEntity struct {
	// What it is called in errors, logs and messages: "category".
	noun string
	// The log attribute its id travels under: "category_id".
	idField string
	// What browsers must re-read after a write: "categories".
	resource string
	// A worked example for the bad-request message.
	example string

	create func(ctx context.Context, id, name string) error
	rename func(ctx context.Context, id, name string) (int64, error)
	remove func(ctx context.Context, id string) (int64, error)
}

// idAndName is the request body of both create and rename; rename
// ignores the id, which comes from the path.
type idAndName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (app *Server) createNamed(entity namedEntity) func(http.ResponseWriter, *http.Request, *UserInfoAdmin) {
	return func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
		ctx, cancel := writeCtx(r.Context())
		defer cancel()

		body, err := decodeBody[idAndName](w, r)
		if err != nil {
			app.apiBadRequest(r, w, entity.example, err)

			return
		}

		// The identifier grammar and the non-empty rule are the
		// database's, enforced by the domains; a malformed id comes
		// back as a 400 through the domain check rather than being
		// restated here.
		if err := entity.create(ctx, body.ID, body.Name); err != nil {
			app.apiDBError(r, w, err, slog.String(entity.idField, body.ID))

			return
		}

		app.logInfo(r, "admin."+entity.resource+".create",
			slog.String("admin_username", aui.Username),
			slog.String(entity.idField, body.ID))
		app.wsHub.Broadcast(WSMessage("invalidate_" + entity.resource))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (app *Server) renameNamed(entity namedEntity) func(http.ResponseWriter, *http.Request, *UserInfoAdmin) {
	return func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
		ctx, cancel := writeCtx(r.Context())
		defer cancel()

		id := r.PathValue("id")

		body, err := decodeBody[struct {
			Name string `json:"name"`
		}](w, r)
		if err != nil {
			app.apiBadRequest(r, w, `expected {"name": "..."}`, err)

			return
		}

		rows, err := entity.rename(ctx, id, body.Name)
		if err != nil {
			app.apiDBError(r, w, err, slog.String(entity.idField, id))

			return
		}

		if rows == 0 {
			app.apiMissing(r, w, entity.noun, slog.String(entity.idField, id))

			return
		}

		app.logInfo(r, "admin."+entity.resource+".rename",
			slog.String("admin_username", aui.Username),
			slog.String(entity.idField, id))
		// Only this resource.
		//
		// A second broadcast used to invalidate the courses too, on the
		// grounds that "course read models embed these names". They do
		// not: v_courses carries category_id, period_ids and
		// allowed_grade_ids and no name at all — name embedding is
		// v_enrollments' convention, not this one. So a rename made
		// every connected browser re-read the entire catalogue, which
		// during a selection window is twelve hundred full reads for a
		// cosmetic change, and repaired nothing that was stale.
		//
		// Both clients already re-read the renamed vocabulary itself on
		// this frame, which is where the new name appears.
		app.wsHub.Broadcast(WSMessage("invalidate_" + entity.resource))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (app *Server) deleteNamed(entity namedEntity) func(http.ResponseWriter, *http.Request, *UserInfoAdmin) {
	return func(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
		ctx, cancel := writeCtx(r.Context())
		defer cancel()

		id := r.PathValue("id")

		// One still referred to is refused by the foreign keys, which
		// is the intended answer: removing it means re-pointing what
		// names it first.
		rows, err := entity.remove(ctx, id)
		if err != nil {
			app.apiDBErrorDeleting(r, w, err, slog.String(entity.idField, id))

			return
		}

		if rows == 0 {
			app.apiMissing(r, w, entity.noun, slog.String(entity.idField, id))

			return
		}

		app.logInfo(r, "admin."+entity.resource+".delete",
			slog.String("admin_username", aui.Username),
			slog.String(entity.idField, id))
		app.wsHub.Broadcast(WSMessage("invalidate_" + entity.resource))
		w.WriteHeader(http.StatusNoContent)
	}
}
