package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Clears one section wholesale for the between-seasons reset. Sections
// still referenced by others fail with a conflict.
func (app *Server) apiDataClear(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	section := r.PathValue("section")

	// Chosen before any transaction is opened, so an unknown section
	// costs nothing. Method expressions, because the queries handle
	// only exists once the transaction does.
	var (
		invalidate string
		deleteAll  func(*db.Queries, context.Context) error
	)

	switch section {
	case "enrollments":
		deleteAll, invalidate = (*db.Queries).DeleteAllEnrollments, "invalidate_enrollments"
	case "students":
		deleteAll, invalidate = (*db.Queries).DeleteAllStudents, "invalidate_students"
	case "courses":
		deleteAll, invalidate = (*db.Queries).DeleteAllCourses, "invalidate_courses"
	case "periods":
		deleteAll, invalidate = (*db.Queries).DeleteAllPeriods, "invalidate_periods"
	case "grades":
		deleteAll, invalidate = (*db.Queries).DeleteAllGrades, "invalidate_grades"
	case "categories":
		deleteAll, invalidate = (*db.Queries).DeleteAllCategories, "invalidate_categories"
	default:
		app.apiError(r, w, http.StatusNotFound, codeNotFound, "unknown section", nil)

		return
	}

	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	tx, err := app.pool.Begin(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("section", section))

		return
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	qtx := app.queries.WithTx(tx)

	// A reset with the enrollment windows still open is a race:
	// clearing enrollments while students can still act just lets them
	// refill, and courses can only be cleared after enrollments, so the
	// gap between the two calls is long enough to matter.
	//
	// Refused rather than repaired. Closing the windows here would mean
	// a delete quietly rewriting the season's schedule — a staggered
	// set of bounds an administrator entered by hand, gone because they
	// cleared a different section. Deleting rows is not a licence to
	// reconfigure, and there is a button for closing.
	open, err := qtx.CountOpenWindows(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("section", section))

		return
	}

	if open > 0 {
		app.apiError(r, w, http.StatusConflict, codeConflict,
			"Enrollment is still open for at least one grade. Close the windows before resetting.", nil,
			slog.String("section", section), slog.Int64("open_grades", open))

		return
	}

	if err := deleteAll(qtx, ctx); err != nil {
		// Deleting, so that a section still referenced by another is
		// reported as what it is. Told the generic way round, the
		// administrator was informed that a reset of the courses
		// "refers to something that does not exist" — which is the
		// message for the opposite failure, and sends them looking for
		// a missing row rather than for the enrollments they have not
		// cleared yet.
		app.apiDBErrorDeleting(r, w, err, slog.String("section", section))

		return
	}

	if err := tx.Commit(ctx); err != nil {
		app.apiDBError(r, w, err, slog.String("section", section))

		return
	}

	app.logInfo(r, logMsgAdminDataClear, slog.String("admin_username", aui.Username), slog.String("section", section))
	app.wsHub.Broadcast(WSMessage(invalidate))
	// Enrollments are the section that can go without taking anything
	// with it; the others are refused by RESTRICT while an enrollment
	// still names them. So whichever section this was, the enrollment
	// table is now empty, and every seat count and derived view moved
	// with it. Nothing marked those courses dirty, because no course
	// was named.
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	app.wsHub.MarkAllDirty()

	if section == "grades" {
		// The grades are gone, and with them every bound the timer
		// could have been armed at.
		app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		app.rearmWindowTimer(r.Context())
	}

	w.WriteHeader(http.StatusNoContent)
}
