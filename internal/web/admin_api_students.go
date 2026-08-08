package web

import (
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Students are upserted, never inserted. One endpoint serves the
// annual roster import and the single-student edit alike, because both
// mean the same thing: make these students be so. It is therefore
// idempotent — re-importing an unchanged file is a no-op, and a file
// with three corrections applies three corrections — which is what
// makes "the export changed, load it again" an ordinary act rather
// than a reset.
//
// Elements are independent: no student competes with another for
// anything, so every malformed row is collected and reported at once
// rather than aborting at the first. An administrator fixing a
// spreadsheet should see every bad row in one pass.
//
// Re-judging is scoped to what actually changed. A changed grade
// re-judges that student's enrollments for the grade rule and their
// budget; a changed legal sex re-judges for the legal-sex rule; a
// changed name, or an unchanged re-imported row, re-judges nothing.
// That is why routine re-imports never resurface violations an
// administrator already accepted.

// studentInput is one element of an upsert.
type studentInput struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	GradeID  string      `json:"grade_id"`
	LegalSex db.LegalSex `json:"legal_sex"`
}

func (app *Server) apiStudentsList(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	students, err := app.queries.GetStudents(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, students)
}

func (app *Server) apiStudentsUpsert(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	body, err := decodeBody[struct {
		Students []studentInput `json:"students"`
		Accept   []string       `json:"accept"`
	}](w, r)
	if err != nil {
		app.apiBadRequest(r, w, `expected {"students": [{"id": "s22537", ...}], "accept": []}`, err)

		return
	}

	if len(body.Students) == 0 {
		app.apiBadRequest(r, w, "students must not be empty", nil)

		return
	}

	if err := app.upsertStudents(r, body.Students, body.Accept); err != nil {
		app.apiDBError(r, w, err, slog.Int("student_count", len(body.Students)))

		return
	}

	app.logInfo(r, logMsgAdminStudentsUpsert,
		slog.String("admin_username", aui.Username),
		slog.Int("student_count", len(body.Students)),
		slog.Int("accepted", len(body.Accept)))
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	// A changed grade changes a student's window, budget and
	// requirements, all of which their own page shows.
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}

// upsertStudents transposes the elements into the parallel arrays the
// function takes. Arrays rather than JSON because students are a flat
// batch: every element has the same four fields, so nothing here is
// ragged and no delimiter has to be invented.
//
// The values go in untyped and are cast per element inside the
// function, each in its own exception scope. That is what lets one bad
// grade id be reported as a bad element rather than aborting the call,
// so the legal_sex is passed as its raw string rather than validated
// here: the database's rejection carries the row index with it, and a
// check here would only produce a worse message earlier.
func (app *Server) upsertStudents(r *http.Request, students []studentInput, accept []string) error {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	ids := make([]string, len(students))
	names := make([]string, len(students))
	gradeIDs := make([]string, len(students))
	legalSexes := make([]string, len(students))

	for i, s := range students {
		ids[i] = s.ID
		names[i] = s.Name
		gradeIDs[i] = s.GradeID
		legalSexes[i] = string(s.LegalSex)
	}

	//nolint:wrapcheck // the caller classifies this by SQLSTATE
	return app.queries.UpsertStudents(ctx, db.UpsertStudentsParams{
		PIds:        ids,
		PNames:      names,
		PGradeIds:   gradeIDs,
		PLegalSexes: legalSexes,
		PAccept:     accept,
	})
}

// Deleting is plain DML, refused by the enrollments foreign key while
// the student holds a seat. That refusal is the intended answer:
// removing someone from the roster means removing them from their
// courses first, deliberately.
func (app *Server) apiStudentsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	ctx, cancel := writeCtx(r.Context())
	defer cancel()

	id := r.PathValue("id")

	rows, err := app.queries.DeleteStudent(ctx, id)
	if err != nil {
		app.apiDBErrorDeleting(r, w, err, slog.String("student_id", id))

		return
	}

	if rows == 0 {
		app.apiMissing(r, w, "student", slog.String("student_id", id))

		return
	}

	app.logInfo(r, logMsgAdminStudentsDelete, slog.String("admin_username", aui.Username), slog.String("student_id", id))
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	w.WriteHeader(http.StatusNoContent)
}
