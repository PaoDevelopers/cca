package web

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

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

// importSeat names one row of an enrollment file.
type importSeat struct {
	CourseID  string `json:"course_id"`
	StudentID string `json:"student_id"`
}

// importViolation is a violation with the row that raised it. The code
// names the course for every rule but budget, so the course is carried
// alongside rather than parsed back out of it.
type importViolation struct {
	apiViolation

	CourseID string `json:"course_id"`
}

// importCheck is what a dry run found: every rule the file would break,
// and every row naming a student or course that does not exist.
type importCheck struct {
	Violations []importViolation `json:"violations"`
	Unknown    []importSeat      `json:"unknown"`
}

// dropRows removes the rows named "course_id:student_id"; neither id
// may contain ':'.
func dropRows(rows [][]string, drop []string) [][]string {
	if len(drop) == 0 {
		return rows
	}

	gone := make(map[string]struct{}, len(drop))
	for _, d := range drop {
		gone[d] = struct{}{}
	}

	return slices.DeleteFunc(rows, func(row []string) bool {
		_, ok := gone[row[0]+":"+row[1]]

		return ok
	})
}

// formList reads a list field sent as lines, one value per line. A
// field per value would run into the multipart parser's 1000-part
// limit on a large draft. Repeated fields still work.
func formList(fields []string) []string {
	var out []string

	for _, f := range fields {
		for line := range strings.Lines(f) {
			if line = strings.TrimRight(line, "\r\n"); line != "" {
				out = append(out, line)
			}
		}
	}

	return out
}

// setAside picks the rows named in later ("course_id:student_id") out of
// the file, with the policy each states, for pending_enrollments. A
// pair the file repeats is kept once, where it first appears.
func setAside(rows [][]string, later []string) (db.InsertPendingEnrollmentsParams, error) {
	//exhaustruct:ignore
	var out db.InsertPendingEnrollmentsParams

	if len(later) == 0 {
		return out, nil
	}

	wanted := make(map[string]struct{}, len(later))
	for _, l := range later {
		wanted[l] = struct{}{}
	}

	for i, row := range rows {
		key := row[0] + ":" + row[1]
		if _, ok := wanted[key]; !ok {
			continue
		}

		delete(wanted, key)

		droppable, err := parseBoolCell(row[2])
		if err != nil {
			return out, rowError(i, "student_droppable: "+err.Error())
		}

		budgeted, err := parseBoolCell(row[3])
		if err != nil {
			return out, rowError(i, "counts_toward_budget: "+err.Error())
		}

		out.CourseIds = append(out.CourseIds, row[0])
		out.StudentIds = append(out.StudentIds, row[1])
		out.Droppable = append(out.Droppable, droppable)
		out.Budgeted = append(out.Budgeted, budgeted)
	}

	return out, nil
}

// unknownRows splits off the rows naming a student or course that does
// not exist, which a placement would refuse outright.
func unknownRows(ctx context.Context, qtx *db.Queries, rows [][]string) ([][]string, []importSeat, error) {
	students, err := qtx.GetStudents(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list students: %w", err)
	}

	courses, err := qtx.GetCourses(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list courses: %w", err)
	}

	knownStudents := make(map[string]struct{}, len(students))
	for _, s := range students {
		knownStudents[s.ID] = struct{}{}
	}

	knownCourses := make(map[string]struct{}, len(courses))
	for _, c := range courses {
		knownCourses[c.ID] = struct{}{}
	}

	var unknown []importSeat

	rows = slices.DeleteFunc(rows, func(row []string) bool {
		_, course := knownCourses[row[0]]
		_, student := knownStudents[row[1]]

		if !course || !student {
			unknown = append(unknown, importSeat{CourseID: row[0], StudentID: row[1]})
		}

		return !course || !student
	})

	return rows, unknown, nil
}

// All-or-nothing, like the course import and for the same reason: half
// an allocation is worse than none.
//
// Six optional form fields make it a draft. check=1 is a dry run that
// reports every conflict instead of stopping at the first; drop names
// rows to leave out; later names rows to leave out for now and keep in
// pending_enrollments, as a draft named by source or else the file's
// name; hold=1 does that with every row drop leaves, for a draft closed
// without deciding, but leaves alone rows another draft holds; accept names the violations the administrator
// chose to keep. A clean file needs none of them.
//
// Unlike the course import, though, the transaction is held here: an
// allocation is a batch of place_enrollments calls, one per course,
// and nothing in the schema wraps them. handleAdmCoursesImport is the
// other shape — upsert_courses holds the whole loop, so that one opens
// no transaction at all. The pointer here used to send a reader to
// that function for the reason this one does the opposite.
//
//nolint:maintidx // one check-or-commit pass over the file; splitting it would scatter the transaction it holds
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

	check := r.MultipartForm.Value["check"] != nil
	accept := formList(r.MultipartForm.Value["accept"])
	drop := formList(r.MultipartForm.Value["drop"])
	later := formList(r.MultipartForm.Value["later"])

	hold := r.MultipartForm.Value["hold"] != nil
	if hold {
		later = nil
		for _, row := range dropRows(slices.Clone(rows), drop) {
			later = append(later, row[0]+":"+row[1])
		}
	}

	// Every pending row the file names is decided by this upload, one
	// way or another — placed, left out, or set aside again — so the
	// file's own rows are what clears them. Cloned: dropRows deletes in
	// place.
	named := slices.Clone(rows)

	pending, err := setAside(named, later)
	if err != nil {
		app.apiBadRequest(r, w, err.Error(), err, slog.String("admin_username", aui.Username))

		return
	}

	// What is set aside waits as a draft named after the file. A resumed
	// draft is rebuilt as CSV, whose name the parser reads the format
	// from, so it says its own name in source instead.
	pending.Source = "upload"
	pending.OnlySource = hold
	if files := r.MultipartForm.File["spreadsheet"]; len(files) > 0 && files[0].Filename != "" {
		pending.Source = files[0].Filename
	}

	if source := r.MultipartForm.Value["source"]; len(source) > 0 && source[0] != "" {
		pending.Source = source[0]
	}

	rows = dropRows(rows, slices.Concat(drop, later))

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

	var found importCheck

	if check {
		rows, found.Unknown, err = unknownRows(ctx, qtx, rows)
		if err != nil {
			app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

			return
		}
	}

	groups, err := groupEnrollmentRows(rows)
	if err != nil {
		app.apiBadRequest(r, w, err.Error(), err, slog.String("admin_username", aui.Username))

		return
	}

	// write runs one placement call. Under check, a refusal is
	// recorded and the call retried accepting exactly what it named,
	// so later courses are judged as if every row had been kept: that
	// is what surfaces both sides of a clash, and every row past a
	// course's capacity, in one pass.
	write := func(courseID string, call func(accept []string) error) error {
		if !check {
			return call(accept)
		}

		sp, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("open savepoint: %w", err)
		}

		err = call(nil)
		if vs := violationsOf(err); vs != nil {
			if err := sp.Rollback(ctx); err != nil {
				return fmt.Errorf("roll back savepoint: %w", err)
			}

			codes := make([]string, len(vs))
			for i, v := range vs {
				codes[i] = v.Code
				found.Violations = append(found.Violations, importViolation{apiViolation: v, CourseID: courseID})
			}

			if sp, err = tx.Begin(ctx); err != nil {
				return fmt.Errorf("reopen savepoint: %w", err)
			}

			err = call(codes)
		}

		if err != nil {
			_ = sp.Rollback(ctx)

			return err
		}

		return sp.Commit(ctx)
	}

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
	//
	// A held file places nothing, so it has nothing to compare.
	var existing []db.VEnrollment
	if len(rows) > 0 {
		existing, err = qtx.GetEnrollments(ctx)
		if err != nil {
			app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

			return
		}
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

		// An import accepts only what the administrator chose to keep in
		// the draft: a placement that breaks a rule is decided on
		// deliberately, not waved through by uploading a file.
		if len(fresh) > 0 {
			if err := write(group.courseID, func(accept []string) error {
				return qtx.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
					PCourseID:           group.courseID,
					PStudentIds:         fresh,
					PStudentDroppable:   group.droppable,
					PCountsTowardBudget: group.budgeted,
					PAccept:             accept,
				})
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
			if err := write(group.courseID, func(accept []string) error {
				return qtx.SetEnrollmentPolicy(ctx, db.SetEnrollmentPolicyParams{
					PCourseID:           group.courseID,
					PStudentIds:         repolicy,
					PStudentDroppable:   group.droppable,
					PCountsTowardBudget: group.budgeted,
					PAccept:             accept,
				})
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

	// A dry run never commits; the deferred rollback undoes it.
	if check {
		app.writeJSON(r, w, found, slog.String("admin_username", aui.Username))

		return
	}

	// In the same transaction as the placements: an upload that fails
	// must not lose the rows it was setting aside, nor clear the ones a
	// previous upload set aside.
	//exhaustruct:ignore
	decided := db.DeletePendingEnrollmentsParams{OnlySource: hold, Source: pending.Source}
	for _, row := range named {
		decided.CourseIds = append(decided.CourseIds, row[0])
		decided.StudentIds = append(decided.StudentIds, row[1])
	}

	if err := qtx.DeletePendingEnrollments(ctx, decided); err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	if err := qtx.DeleteSettledPending(ctx); err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	var kept int64

	if len(pending.CourseIds) > 0 {
		if kept, err = qtx.InsertPendingEnrollments(ctx, pending); err != nil {
			app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	app.logInfo(r, logMsgAdminEnrollmentsImport,
		slog.String("admin_username", aui.Username), slog.Int("row_count", len(rows)),
		slog.Int64("pending_count", kept))
	app.wsHub.BroadcastToStudentsAndAdmins(slices.Collect(maps.Keys(students)),
		WSMessage("invalidate_enrollments"))
	app.broadcastCourseCounts(courses)

	// An upload that set rows aside says how many were kept, which can
	// be fewer than it named: none, when every row named a student or
	// course that does not exist, a seat already taken, or one waiting
	// in another draft, and the administrator should hear that rather
	// than look for a draft.
	if hold || len(later) > 0 {
		app.writeJSON(r, w, struct {
			Kept int64 `json:"kept"`
		}{Kept: kept}, slog.String("admin_username", aui.Username))

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
