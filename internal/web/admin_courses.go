package web

import (
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The course spreadsheet is the CCA department's master list, and
// importing it is how a season starts. Its columns are the course
// form's fields, with the three list-valued ones as comma-separated
// cells.
//
//nolint:gochecknoglobals
var courseImportColumns = []string{
	"id",
	"name",
	"description",
	"periods",
	"max_students",
	"invite_only",
	"teacher",
	"teacher_email",
	"location",
	"term",
	"cost",
	"category",
	"allowed_legal_sexes",
	"allowed_grades",
}

// Column indices, named so the parser below reads as the spreadsheet
// does rather than as a sequence of magic subscripts.
const (
	courseColID = iota
	courseColName
	courseColDescription
	courseColPeriods
	courseColMaxStudents
	courseColInviteOnly
	courseColTeacher
	courseColTeacherEmail
	courseColLocation
	courseColTerm
	courseColCost
	courseColCategory
	courseColLegalSexes
	courseColGrades
)

// courseElement is one element of an upsert_courses batch. The field
// names are the function's, and the whole batch travels as JSON
// because a course is ragged: each names its own period set and its
// own two restriction sets.
type courseElement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CategoryID  string `json:"category_id"`

	Teacher      string `json:"teacher"`
	TeacherEmail string `json:"teacher_email"`
	Location     string `json:"location"`
	Term         string `json:"term"`
	Cost         string `json:"cost"`

	// Both travel as the spreadsheet's own text, uncast. PostgreSQL
	// reads every boolean spelling a spreadsheet produces — TRUE,
	// true, yes, y, 1 and their negatives, case-insensitively — and
	// an empty cell is turned into false by the function. Casting
	// them here instead would move two more failure modes out of the
	// collect-everything path and into a first-error one.
	InviteOnly  string `json:"invite_only"`
	MaxStudents string `json:"max_students"`

	PeriodIDs  []string `json:"period_ids"`
	LegalSexes []string `json:"legal_sexes"`
	GradeIDs   []string `json:"grade_ids"`
}

// A row of the course spreadsheet, transposed and not judged.
//
// Nothing is validated here at all. The identifier grammars, the
// non-empty rules, the enum values, the foreign keys and the two casts
// all belong to the database, which reports them by element index
// through YKD01 — so a file with six mistakes comes back naming all
// six. Anything checked here instead would be a second copy to keep in
// step, and would reject on the first bad row.
func courseRowElement(row []string) courseElement {
	return courseElement{
		ID:           row[courseColID],
		Name:         row[courseColName],
		Description:  row[courseColDescription],
		CategoryID:   row[courseColCategory],
		Teacher:      row[courseColTeacher],
		TeacherEmail: row[courseColTeacherEmail],
		Location:     row[courseColLocation],
		Term:         row[courseColTerm],
		Cost:         row[courseColCost],
		InviteOnly:   row[courseColInviteOnly],
		MaxStudents:  row[courseColMaxStudents],
		PeriodIDs:    splitList(row[courseColPeriods]),
		LegalSexes:   splitList(row[courseColLegalSexes]),
		GradeIDs:     splitList(row[courseColGrades]),
	}
}

// One call, whatever the size of the file, and no transaction opened
// here: upsert_courses holds the loop, so the whole spreadsheet lands
// or none of it does, and the lock order is taken over the batch
// rather than in the order the file happens to be sorted.
//
// It is an upsert for the same reason the roster import is. The master
// list is edited and re-loaded through a season; absence from a file
// is not evidence a course was cancelled, and re-loading an unchanged
// file must change nothing.
func (app *Server) handleAdmCoursesImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesImport", slog.String("admin_username", aui.Username))

	if r.Method != http.MethodPost {
		app.apiMethodNotAllowed(r, w, slog.String("admin_username", aui.Username))

		return
	}

	rows, err := readSpreadsheetUpload(w, r, courseImportColumns)
	if err != nil {
		app.spreadsheetUploadError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	elements := make([]courseElement, len(rows))
	for i, row := range rows {
		elements[i] = courseRowElement(row)
	}

	// An import accepts nothing: a course edit that would break an
	// existing placement is reported so an administrator decides on it
	// deliberately, through the course form, rather than being waved
	// through by the act of uploading a file.
	if err := app.upsertCourses(r, elements, nil); err != nil {
		app.apiDBError(r, w, err,
			slog.String("admin_username", aui.Username), slog.Int("row_count", len(elements)))

		return
	}

	app.logInfo(r, logMsgAdminCoursesImport,
		slog.String("admin_username", aui.Username), slog.Int("row_count", len(elements)))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	// A rescheduled or re-restricted course changes what its enrollees
	// occupy and what everyone else may join.
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}

// upsertCourses encodes the batch and makes the call. The payload is
// re-encoded from decoded values rather than forwarded from the
// request, so nothing unvalidated reaches the database.
func (app *Server) upsertCourses(r *http.Request, elements []courseElement, accept []string) error {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	payload, err := json.Marshal(elements)
	if err != nil {
		return fmt.Errorf("encode course batch: %w", err)
	}

	//nolint:wrapcheck // the caller classifies this by SQLSTATE
	return app.queries.UpsertCourses(ctx, db.UpsertCoursesParams{
		PCourses: payload,
		PAccept:  accept,
	})
}
