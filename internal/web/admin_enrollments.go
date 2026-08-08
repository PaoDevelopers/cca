package web

import (
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The enrollment spreadsheet, in and out.
//
// These are the one pair that does not match, and deliberately so. Out
// it is the PowerSchool hand-off, so it carries names and terms nobody
// imports and repeats an enrollment once per period the course meets
// in — that is the shape the other system wants. In, only four columns
// decide anything.
//
// An administrator has no reason to know that. They export, edit in a
// spreadsheet, and upload the file they were handed, so the import
// accepts either shape and reduces the wide one: the columns it does
// not need are dropped, and the repeated rows for one enrollment
// collapse back into the one enrollment they describe.
//
//nolint:gochecknoglobals
var enrollmentImportColumns = []string{
	"course_id",
	"student_id",
	"student_droppable",
	"counts_toward_budget",
}

// The export's own columns, accepted on the way back in.
//
//nolint:gochecknoglobals
var enrollmentExportColumns = []string{
	"student_id", "student_name", "grade", "legal_sex",
	"course_id", "course_name", "term", "period",
	"student_droppable", "counts_toward_budget",
}

func (app *Server) handleAdmEnrollmentsExport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmEnrollmentsExport", slog.String("admin_username", aui.Username))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w, slog.String("admin_username", aui.Username))

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	rows, err := app.queries.GetEnrollmentsExport(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	records := [][]string{enrollmentExportColumns}

	for _, row := range rows {
		records = append(records, []string{
			row.StudentID,
			row.StudentName,
			row.GradeID,
			string(row.LegalSex),
			row.CourseID,
			row.CourseName,
			row.Term,
			row.PeriodID,
			strconv.FormatBool(row.StudentDroppable),
			strconv.FormatBool(row.CountsTowardBudget),
		})
	}

	if app.writeCSV(w, r, "enrollments.csv", records) {
		app.logInfo(r, logMsgAdminEnrollmentsExport,
			slog.String("admin_username", aui.Username), slog.Int("row_count", len(rows)))
	}
}

// placement groups the rows of an import by course, because
// place_enrollments takes one course and many students: that is the
// unit it can lock and judge as a whole, and the unit whose batch
// order is significant.
type placement struct {
	courseID   string
	studentIDs []string
	droppable  bool
	budgeted   bool
}

// groupEnrollmentRows collects consecutive rows sharing a course and
// policy. Order within a course is preserved, because placement is an
// ordered batch: students compete for the same seats and earlier rows
// win when a course fills mid-batch.
//
// Groups are then sorted by course id. Each place_enrollments call
// locks its own course, so a transaction making several of them takes
// course locks in the order it makes the calls; one order for every
// such transaction is what stops two concurrent imports deadlocking.
func groupEnrollmentRows(rows [][]string) ([]placement, error) {
	var groups []placement

	for i, row := range rows {
		droppable, err := parseBoolCell(row[2])
		if err != nil {
			return nil, rowError(i, "student_droppable: "+err.Error())
		}

		budgeted, err := parseBoolCell(row[3])
		if err != nil {
			return nil, rowError(i, "counts_toward_budget: "+err.Error())
		}

		last := len(groups) - 1
		if last >= 0 && groups[last].courseID == row[0] &&
			groups[last].droppable == droppable && groups[last].budgeted == budgeted {
			groups[last].studentIDs = append(groups[last].studentIDs, row[1])

			continue
		}

		groups = append(groups, placement{
			courseID:   row[0],
			studentIDs: []string{row[1]},
			droppable:  droppable,
			budgeted:   budgeted,
		})
	}

	slices.SortStableFunc(groups, func(a, b placement) int {
		switch {
		case a.courseID < b.courseID:
			return -1
		case a.courseID > b.courseID:
			return 1
		default:
			return 0
		}
	})

	return groups, nil
}

// reduceExportRows turns the export's ten columns into the import's
// four, collapsing the rows that describe one enrollment.
//
// The export repeats an enrollment once per period its course meets
// in, and emits one row with an empty period for a course with none,
// so the same (course, student) appears between one and several times.
// They are one enrollment and must be placed once — placing it twice
// is a duplicate key, and an import that fails on a file the export
// produced is not a round trip.
//
// Order is the file's, which matters: placement is an ordered batch,
// and when a course fills mid-batch the earlier rows win. Only the
// first occurrence of a pair is kept, so the position it had in the
// file is the position it keeps.
func reduceExportRows(rows [][]string) [][]string {
	const (
		studentID          = 0
		courseID           = 4
		studentDroppable   = 8
		countsTowardBudget = 9
	)

	type pair struct{ course, student string }

	seen := make(map[pair]struct{}, len(rows))
	out := make([][]string, 0, len(rows))

	for _, row := range rows {
		key := pair{course: row[courseID], student: row[studentID]}
		if _, repeat := seen[key]; repeat {
			continue
		}

		seen[key] = struct{}{}

		out = append(out, []string{
			row[courseID],
			row[studentID],
			row[studentDroppable],
			row[countsTowardBudget],
		})
	}

	return out
}

// All-or-nothing, like the course import and for the same reason: half
// an allocation is worse than none.
//
// Unlike the course import, though, the transaction is held here: an
// allocation is a batch of place_enrollments calls, one per course,
// and nothing in the schema wraps them. handleAdmCoursesImport is the
// other shape — upsert_courses holds the whole loop, so that one opens
// no transaction at all. The pointer here used to send a reader to
// that function for the reason this one does the opposite.
func (app *Server) handleAdmEnrollmentsImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmEnrollmentsImport", slog.String("admin_username", aui.Username))

	if r.Method != http.MethodPost {
		app.apiMethodNotAllowed(r, w, slog.String("admin_username", aui.Username))

		return
	}

	rows, shape, err := readSpreadsheetUploadAny(w, r,
		enrollmentImportColumns, enrollmentExportColumns)
	if err != nil {
		app.spreadsheetUploadError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	if shape == 1 {
		rows = reduceExportRows(rows)
	}

	groups, err := groupEnrollmentRows(rows)
	if err != nil {
		app.apiBadRequest(r, w, err.Error(), err, slog.String("admin_username", aui.Username))

		return
	}

	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	tx, err := app.pool.Begin(ctx)
	if err != nil {
		app.apiInternalError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	qtx := app.queries.WithTx(tx)

	// What is already there, so that re-uploading a file changes only
	// what the file changed.
	//
	// Placement is an insert, so without this an unedited re-import is
	// a duplicate key on its first already-placed row and the whole
	// upload fails — which is not a thing an administrator can be
	// expected to reason about, given that both other imports are
	// upserts and re-uploading is how one fixes a typo in row 400.
	//
	// So each named enrollment goes one of three ways: it does not
	// exist and is placed, it exists with the policy the file states
	// and is left alone, or it exists with a different policy and the
	// file's policy is applied. The last is the file stating the
	// arrangement it wants, exactly as the course and roster imports
	// do; nothing is silently overwritten that the file did not name.
	existing, err := qtx.GetEnrollments(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	type seat struct{ course, student string }

	current := make(map[seat]db.VEnrollment, len(existing))
	for _, e := range existing {
		current[seat{course: e.CourseID, student: e.StudentID}] = e
	}

	students := make(map[string]struct{}, len(rows))
	courses := make([]string, 0, len(groups))

	for _, group := range groups {
		var fresh, repolicy []string

		for _, id := range group.studentIDs {
			held, already := current[seat{course: group.courseID, student: id}]

			switch {
			case !already:
				fresh = append(fresh, id)
			case held.StudentDroppable != group.droppable ||
				held.CountsTowardBudget != group.budgeted:
				repolicy = append(repolicy, id)
			}
		}

		// An import accepts nothing, as with the roster: a placement
		// that breaks a rule is reported so it can be decided on
		// deliberately, not waved through by uploading a file.
		if len(fresh) > 0 {
			if err := qtx.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
				PCourseID:           group.courseID,
				PStudentIds:         fresh,
				PStudentDroppable:   group.droppable,
				PCountsTowardBudget: group.budgeted,
				PAccept:             nil,
			}); err != nil {
				app.apiDBError(r, w, err,
					slog.String("admin_username", aui.Username),
					slog.String("course_id", group.courseID))

				return
			}
		}

		// Same course, so the same lock, taken again in the same
		// order. Nothing about the sequence changes.
		if len(repolicy) > 0 {
			if err := qtx.SetEnrollmentPolicy(ctx, db.SetEnrollmentPolicyParams{
				PCourseID:           group.courseID,
				PStudentIds:         repolicy,
				PStudentDroppable:   group.droppable,
				PCountsTowardBudget: group.budgeted,
				PAccept:             nil,
			}); err != nil {
				app.apiDBError(r, w, err,
					slog.String("admin_username", aui.Username),
					slog.String("course_id", group.courseID))

				return
			}
		}

		if len(fresh) == 0 && len(repolicy) == 0 {
			continue
		}

		courses = append(courses, group.courseID)

		for _, id := range group.studentIDs {
			students[id] = struct{}{}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	app.logInfo(r, logMsgAdminEnrollmentsImport,
		slog.String("admin_username", aui.Username), slog.Int("row_count", len(rows)))
	app.wsHub.BroadcastToStudentsAndAdmins(slices.Collect(maps.Keys(students)),
		WSMessage("invalidate_enrollments"))
	app.broadcastCourseCounts(courses)
	w.WriteHeader(http.StatusNoContent)
}
