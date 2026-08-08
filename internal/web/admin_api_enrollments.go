package web

import (
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Administrators act on one course's enrollments at a time, because
// that is the unit the database can judge and lock as a whole. A
// request naming several courses would be several intentions wearing
// one name, and its partial failure would have nowhere honest to be
// reported.
//
// The three operations differ in exactly what they can break:
//
//	place    every negotiable rule, per student
//	policy   the budget rule, per student, and only when charging
//	remove   nothing; removal is monotone
//
// so the first two take an accept list and the third does not. None of
// them opens a transaction here: each database function locks its
// course, then its students, in the one global order, so concurrent
// administrators queue rather than deadlock.

func (app *Server) apiEnrollmentsList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	enrollments, err := app.queries.GetEnrollments(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, enrollments)
}

// placementInput is shared by place and policy: both name a course, a
// set of students, and the policy the resulting enrollments carry.
type placementInput struct {
	CourseID   string   `json:"course_id"`
	StudentIDs []string `json:"student_ids"`

	// The two bits, spelled out rather than encoded as a type name.
	// All four combinations are legal and mean different things; the
	// old three-valued enum could not express "the student's own
	// committed pick".
	StudentDroppable   bool `json:"student_droppable"`
	CountsTowardBudget bool `json:"counts_toward_budget"`

	Accept []string `json:"accept"`
}

func (in placementInput) validate() string {
	if in.CourseID == "" {
		return "course_id must not be empty"
	}

	if len(in.StudentIDs) == 0 {
		return "student_ids must not be empty"
	}

	return ""
}

func (app *Server) apiEnrollmentsPlace(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	in, err := decodeBody[placementInput](w, r)
	if err != nil {
		app.apiBadRequest(r, w,
			`expected {"course_id": "...", "student_ids": [...], "student_droppable": bool, "counts_toward_budget": bool}`, err)

		return
	}

	if message := in.validate(); message != "" {
		app.apiBadRequest(r, w, message, nil)

		return
	}

	// Deliberately not sorted or deduplicated. Students compete for
	// the same seats and can clash with each other, so the batch is
	// ordered and earlier elements win; reordering it would change
	// which students get in when the course fills mid-batch.
	if err := app.queries.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID:           in.CourseID,
		PStudentIds:         in.StudentIDs,
		PStudentDroppable:   in.StudentDroppable,
		PCountsTowardBudget: in.CountsTowardBudget,
		PAccept:             in.Accept,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("course_id", in.CourseID), slog.Int("student_count", len(in.StudentIDs)))

		return
	}

	app.logInfo(r, logMsgAdminEnrollmentsPlace,
		slog.String("admin_username", aui.Username),
		slog.String("course_id", in.CourseID),
		slog.Any("student_ids", in.StudentIDs),
		slog.Bool("student_droppable", in.StudentDroppable),
		slog.Bool("counts_toward_budget", in.CountsTowardBudget),
		slog.Int("accepted", len(in.Accept)))
	app.wsHub.BroadcastToStudentsAndAdmins(in.StudentIDs, WSMessage("invalidate_enrollments"))
	app.broadcastCourseCounts([]string{in.CourseID})
	w.WriteHeader(http.StatusNoContent)
}

// Re-policy without releasing the seats: an invitation becomes a
// placement the student may not decline, or a placement starts (or
// stops) charging their budget. Remove-and-replace would do neither
// atomically and would give up the seat in between.
func (app *Server) apiEnrollmentsPolicy(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	in, err := decodeBody[placementInput](w, r)
	if err != nil {
		app.apiBadRequest(r, w,
			`expected {"course_id": "...", "student_ids": [...], "student_droppable": bool, "counts_toward_budget": bool}`, err)

		return
	}

	if message := in.validate(); message != "" {
		app.apiBadRequest(r, w, message, nil)

		return
	}

	if err := app.queries.SetEnrollmentPolicy(ctx, db.SetEnrollmentPolicyParams{
		PCourseID:           in.CourseID,
		PStudentIds:         in.StudentIDs,
		PStudentDroppable:   in.StudentDroppable,
		PCountsTowardBudget: in.CountsTowardBudget,
		PAccept:             in.Accept,
	}); err != nil {
		app.apiDBError(r, w, err, slog.String("course_id", in.CourseID), slog.Int("student_count", len(in.StudentIDs)))

		return
	}

	app.logInfo(r, logMsgAdminEnrollmentsPlace,
		slog.String("admin_username", aui.Username),
		slog.String("course_id", in.CourseID),
		slog.Any("student_ids", in.StudentIDs),
		slog.Bool("student_droppable", in.StudentDroppable),
		slog.Bool("counts_toward_budget", in.CountsTowardBudget),
		slog.Int("accepted", len(in.Accept)))
	app.wsHub.BroadcastToStudentsAndAdmins(in.StudentIDs, WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}

// Removal is monotone: it can only shrink the violation set, so there
// is nothing to accept and nothing to re-judge. The droppability bit
// binds students, not administrators, so a forced placement comes out
// here like any other.
func (app *Server) apiEnrollmentsRemove(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	body, err := decodeBody[struct {
		CourseID   string   `json:"course_id"`
		StudentIDs []string `json:"student_ids"`
	}](w, r)
	if err != nil || body.CourseID == "" || len(body.StudentIDs) == 0 {
		app.apiBadRequest(r, w, `expected {"course_id": "...", "student_ids": [...]}`, err)

		return
	}

	if err := app.queries.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
		PCourseID:   body.CourseID,
		PStudentIds: body.StudentIDs,
	}); err != nil {
		app.apiDBErrorDeleting(r, w, err, slog.String("course_id", body.CourseID), slog.Int("student_count", len(body.StudentIDs)))

		return
	}

	app.logInfo(r, logMsgAdminEnrollmentsRemove,
		slog.String("admin_username", aui.Username),
		slog.String("course_id", body.CourseID),
		slog.Any("student_ids", body.StudentIDs))
	app.wsHub.BroadcastToStudentsAndAdmins(body.StudentIDs, WSMessage("invalidate_enrollments"))
	app.broadcastCourseCounts([]string{body.CourseID})
	w.WriteHeader(http.StatusNoContent)
}
