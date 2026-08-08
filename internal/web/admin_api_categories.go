package web

import (
	"context"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Categories are the simplest thing in the system: an id, a name, and
// no rules. Every operation is one statement, which is why none of
// them goes through a write function — and why the writes are the
// shared vocabulary handlers rather than three of their own.

func (app *Server) categories() namedEntity {
	return namedEntity{
		noun:     "category",
		idField:  "category_id",
		resource: "categories",
		example:  `expected {"id": "SPORT", "name": "Sports"}`,

		create: func(ctx context.Context, id, name string) error {
			return app.queries.NewCategory(ctx, db.NewCategoryParams{ID: id, Name: name})
		},
		rename: func(ctx context.Context, id, name string) (int64, error) {
			return app.queries.RenameCategory(ctx, db.RenameCategoryParams{ID: id, Name: name})
		},
		remove: func(ctx context.Context, id string) (int64, error) {
			return app.queries.DeleteCategory(ctx, id)
		},
	}
}

func (app *Server) apiCategoriesList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	categories, err := app.queries.GetCategories(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, categories)
}
