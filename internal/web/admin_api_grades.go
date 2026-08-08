package web

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Grade writes are split by what they can break, not by what they
// touch. Only one of them changes an input to a negotiable rule — the
// budget cap — and it is the only one that takes an accept list.
// Everything else is plain DML that cannot make an existing enrollment
// wrong, so nothing else knows the accept protocol exists.

func (app *Server) apiGradesList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	rows, err := app.Grades(ctx)
	if err != nil {
		app.apiInternalError(r, w, err)

		return
	}

	app.writeJSON(r, w, rows)
}

func (app *Server) apiGradesCreate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	body, err := decodeBody[struct {
		ID                    string `json:"id"`
		Name                  string `json:"name"`
		MaxBudgetedPeriods    *int64 `json:"max_budgeted_periods"`
		MinDistinctCategories int64  `json:"min_distinct_categories"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"id": "Y9", "name": "Year 9", ...}`, err)

		return
	}

	// A new grade is created closed: both bounds NULL. Opening it is
	// a separate, deliberate act.
	if err := app.queries.NewGrade(ctx, db.NewGradeParams{
		ID:                    body.ID,
		Name:                  body.Name,
		MaxBudgetedPeriods:    pgInt64(body.MaxBudgetedPeriods),
		MinDistinctCategories: body.MinDistinctCategories,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", body.ID))

		return
	}

	app.logInfo(r, logMsgAdminGradesCreate, slog.String("admin_username", aui.Username), slog.String("grade_id", body.ID))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	w.WriteHeader(http.StatusNoContent)
}

// Name and the advisory minimum: neither is any rule's input, so
// neither can invalidate an enrollment.
//
// The minimum is still an input to something. requirements_met, on
// every row of the admin's student table, is
// distinct_categories_used >= min_distinct_categories and every
// requirement met — so raising the minimum makes students stop
// satisfying it. See the broadcast at the end.
func (app *Server) apiGradesUpdate(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	body, err := decodeBody[struct {
		Name                  string `json:"name"`
		MinDistinctCategories int64  `json:"min_distinct_categories"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"name": "Year 9", "min_distinct_categories": 2}`, err)

		return
	}

	// One statement, so an edit cannot half-land: the name and the
	// minimum are one form and one save.
	rows, err := app.queries.UpdateGradeSettings(ctx, db.UpdateGradeSettingsParams{
		ID: id, Name: body.Name, MinDistinctCategories: body.MinDistinctCategories,
	})
	if err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	if rows == 0 {
		app.apiMissing(r, w, "grade", slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesUpdate, slog.String("admin_username", aui.Username), slog.String("grade_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	// Whether a student satisfies their requirements is computed
	// against min_distinct_categories, so this write moved it. Without
	// this the administrator's own Students page went on saying "1/4
	// OK" about students the server was already reporting as not met —
	// and the student's page, which does re-read on a grade frame,
	// said the opposite at the same moment.
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	w.WriteHeader(http.StatusNoContent)
}

// The enrollment window. Both bounds are nullable and mean what their
// absence says: no opens_at is a closed window, no closes_at is one
// that stays open until someone closes it.
//
// Nothing is scheduled to fire at either bound. Openness is derived
// wherever it is read, so a window that passes while the server is
// down is simply open or closed the next time anyone asks. The timer
// re-armed here exists only so that a browser already looking at the
// page repaints at the boundary rather than at its next refetch.
// decodeBound reads one window bound out of a request body, reporting
// whether it was there at all.
//
// Absent is nil, and there is nothing to write. Present and null is
// the bound being removed. Anything else must be an instant.
func decodeBound(raw jsontext.Value) (*time.Time, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}

	if string(raw) == "null" {
		return nil, true, nil
	}

	var at time.Time
	if err := json.Unmarshal(raw, &at); err != nil {
		return nil, false, fmt.Errorf("read a window bound: %w", err)
	}

	return &at, true, nil
}

func (app *Server) apiGradesWindow(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	// Three states per bound, not two: absent leaves it alone, null
	// clears it, an instant sets it. NULL is a value here — it is what
	// "no such bound" is spelt as — so absence needs a spelling of its
	// own, and jsontext.Value is the one that distinguishes them.
	//
	// The card's two boxes are edited separately and by different
	// people, so a write that named both would carry back whatever the
	// untouched box was built with. That is the value the grade had
	// when the page loaded, which is not the value it has now if
	// somebody else moved it — and restoring it looks like nothing
	// happened, because the write succeeds and the value is one that
	// had genuinely been there.
	body, err := decodeBody[struct {
		OpensAt  jsontext.Value `json:"opens_at"`
		ClosesAt jsontext.Value `json:"closes_at"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w,
			`expected {"opens_at": "2026-09-01T08:00:00Z"} or {"closes_at": null}`, err)

		return
	}

	opens, setOpens, err := decodeBound(body.OpensAt)
	if err != nil {
		app.apiBadRequest(r, w, "opens_at must be an RFC 3339 instant or null", err)

		return
	}

	closes, setCloses, err := decodeBound(body.ClosesAt)
	if err != nil {
		app.apiBadRequest(r, w, "closes_at must be an RFC 3339 instant or null", err)

		return
	}

	if !setOpens && !setCloses {
		app.apiBadRequest(r, w, "name at least one of opens_at and closes_at", nil)

		return
	}

	rows, err := app.queries.SetGradeWindow(ctx, db.SetGradeWindowParams{
		ID:          id,
		SetOpensAt:  setOpens,
		OpensAt:     pgTime(opens),
		SetClosesAt: setCloses,
		ClosesAt:    pgTime(closes),
	})
	if err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	if rows == 0 {
		app.apiMissing(r, w, "grade", slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesWindow,
		slog.String("admin_username", aui.Username), slog.String("grade_id", id),
		slog.Any("opens_at", opens), slog.Any("closes_at", closes))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.rearmWindowTimer(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// The budget cap is the one grade field that is an input to a
// negotiable rule, so it is the one that can come back with
// violations: lowering it can put students already enrolled over the
// new cap. set_max_budgeted_periods applies the change, re-judges
// every student of the grade, and raises unless each is accepted.
func (app *Server) apiGradesBudget(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	body, err := decodeBody[struct {
		MaxBudgetedPeriods *int64   `json:"max_budgeted_periods"`
		Accept             []string `json:"accept"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"max_budgeted_periods": 4, "accept": []}`, err)

		return
	}

	if err := app.queries.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
		GradeID:            id,
		MaxBudgetedPeriods: pgInt64(body.MaxBudgetedPeriods),
		Accept:             body.Accept,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesBudget,
		slog.String("admin_username", aui.Username), slog.String("grade_id", id),
		slog.Any("max_budgeted_periods", body.MaxBudgetedPeriods),
		slog.Int("accepted", len(body.Accept)))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	// A student's remaining budget is part of their status document.
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	w.WriteHeader(http.StatusNoContent)
}

// Declarative, like the period order, and for the same reasons.
func (app *Server) apiGradesOrder(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	body, err := decodeBody[struct {
		IDs []string `json:"ids"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"ids": ["Y9", "Y10"]}`, err)

		return
	}

	if err := app.queries.SetGradeOrder(ctx, body.IDs); err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.logInfo(r, logMsgAdminGradesOrder, slog.String("admin_username", aui.Username), slog.Int("grade_count", len(body.IDs)))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	w.WriteHeader(http.StatusNoContent)
}

// A grade's requirements are one form and one save, so they are
// replaced as a whole rather than created and deleted one at a time.
// Requirements are advisory: replacing them can change what a student
// is told and nothing else, so there is no accept list here.
func (app *Server) apiGradesRequirements(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	// A pointer, so that a missing or null count is distinguishable
	// from zero. It decodes to zero otherwise, and zero is a
	// meaningful value here — a requirement of zero periods is
	// permanently met — so a client that forgot to send one would
	// silently turn a real requirement into a satisfied one.
	body, err := decodeBody[struct {
		Requirements []struct {
			MinPeriodCount *int64   `json:"min_period_count"`
			CategoryIDs    []string `json:"category_ids"`
		} `json:"requirements"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w,
			`expected {"requirements": [{"min_period_count": 2, "category_ids": ["SPORT"]}]}`, err)

		return
	}

	for i, requirement := range body.Requirements {
		if requirement.MinPeriodCount == nil {
			app.apiBadRequest(r, w,
				"requirement "+strconv.Itoa(i+1)+" does not say how many periods it requires",
				nil, slog.String("grade_id", id))

			return
		}

		if *requirement.MinPeriodCount < 0 {
			app.apiBadRequest(r, w,
				"requirement "+strconv.Itoa(i+1)+" requires a negative number of periods",
				nil, slog.String("grade_id", id))

			return
		}
	}

	// The function takes JSONB because requirements are ragged; this
	// re-encodes the decoded body rather than forwarding the raw
	// request, so nothing unvalidated reaches the database.
	payload, err := json.Marshal(body.Requirements)
	if err != nil {
		app.apiInternalError(r, w, err, slog.String("grade_id", id))

		return
	}

	if err := app.queries.SetGradeRequirements(ctx, db.SetGradeRequirementsParams{
		PGradeID:      id,
		PRequirements: payload,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesRequirements,
		slog.String("admin_username", aui.Username), slog.String("grade_id", id),
		slog.Int("requirement_count", len(body.Requirements)))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	// Requirement satisfaction is per student and is exactly what
	// this rewrote, so the standing view of it is now stale.
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *Server) apiGradesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	rows, err := app.queries.DeleteGrade(ctx, id)
	if err != nil {
		app.apiDBErrorDeleting(r, w, err, slog.String("grade_id", id))

		return
	}

	if rows == 0 {
		app.apiMissing(r, w, "grade", slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesDelete, slog.String("admin_username", aui.Username), slog.String("grade_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.rearmWindowTimer(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// The two manual levers. They take no body: their whole argument is
// "now", and the caller is the one party that must not supply it.
//
// A browser reading its own clock made three clocks in the system —
// the database's, which decides openness; the Go process's, which arms
// the repaint timer; and the administrator's laptop, which wrote the
// value the other two then interpreted. Worse, it arrived rounded to
// the minute, that being all a datetime-local box carries, so opening
// and closing inside one minute wrote closes_at = opens_at and was
// refused by the CHECK — an error about a value nobody typed, with the
// window still open. Reading the clock in the statement that writes it
// removes the round trip and the rounding together.
//
// The scheduled bounds remain the real interface; these are shortcuts
// through it, not a second way of saying when a window runs.
func (app *Server) apiGradesOpenNow(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	res, err := app.queries.OpenGradeWindowNow(ctx, id)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	if !res.Found {
		app.apiMissing(r, w, "grade", slog.String("grade_id", id))

		return
	}

	// Opening leaves a scheduled closing time alone, which is the
	// point of it — but a closing time already behind us cannot be
	// kept and opened around, because the row would say the window
	// shuts before it starts. Refusing names the one edit that
	// resolves it; opening anyway would mean discarding a bound the
	// administrator set, silently, which is what this stopped doing.
	if !res.Opened {
		app.apiError(r, w, http.StatusConflict, codeConflict,
			"That grade's closing time has already passed. Clear or move it before opening the window.",
			nil, slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesOpenNow,
		slog.String("admin_username", aui.Username), slog.String("grade_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.rearmWindowTimer(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// As apiGradesOpenNow, for the closing lever.
//
// Zero rows is not a missing grade here: it is a grade whose window
// was not running, which is a different thing to report and not an
// error. Closing what is already shut is the administrator getting
// what they asked for.
func (app *Server) apiGradesCloseNow(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	closed, err := app.queries.CloseGradeWindowNow(ctx, id)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("grade_id", id))

		return
	}

	app.logInfo(r, logMsgAdminGradesCloseNow,
		slog.String("admin_username", aui.Username), slog.String("grade_id", id),
		slog.Int64("closed", closed))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.rearmWindowTimer(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// Shuts every window that is currently open, in one call.
//
// This exists because the alternative is a per-grade walk at the one
// moment it must not be: the end of a selection season, with the
// grades staggered, an administrator clicking six times while students
// are still acting in the grades not yet reached. It is also what a
// reset asks for after refusing — see apiDataClear.
//
// Idempotent by construction: it names the rows that are open right
// now, so calling it twice closes nothing the second time.
func (app *Server) apiGradesCloseAll(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	closed, err := app.queries.CloseOpenWindows(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.logInfo(r, logMsgAdminGradesCloseAll,
		slog.String("admin_username", aui.Username), slog.Int64("closed", closed))
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	// Students holding the page have their buttons replaced by the
	// closed state; nothing they could still do survives this.
	app.rearmWindowTimer(r.Context())
	w.WriteHeader(http.StatusNoContent)
}
