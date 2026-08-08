package web

import (
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The roster spreadsheet, as PowerSchool exports it. The id column is
// the school email's localpart ("s22537"), which is what the database
// stores and what a student's sign-in produces, so nothing has to be
// mapped between the roster and the session.
//
//nolint:gochecknoglobals
var studentImportColumns = []string{"id", "name", "grade", "legal_sex"}

// One call, whatever the size of the file. upsert_students takes the
// four columns as parallel arrays and casts each element in its own
// exception scope, so the malformed rows come back together as one
// YKD01 payload rather than one per upload; and because it is an
// upsert, re-importing a file that has already been loaded changes
// nothing.
//
// Nothing is validated here beyond the shape of the file. Every
// element rejection the database can make carries the element's index
// with it, and an index is exactly what a spreadsheet editor needs.
func (app *Server) handleAdmStudentsImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsImport", slog.String("admin_username", aui.Username))

	if r.Method != http.MethodPost {
		app.apiMethodNotAllowed(r, w, slog.String("admin_username", aui.Username))

		return
	}

	rows, err := readSpreadsheetUpload(w, r, studentImportColumns)
	if err != nil {
		app.spreadsheetUploadError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	if len(rows) == 0 {
		app.apiBadRequest(r, w, "the file has a header but no rows", nil, slog.String("admin_username", aui.Username))

		return
	}

	students := make([]studentInput, len(rows))
	for i, row := range rows {
		students[i] = studentInput{
			ID:       row[0],
			Name:     row[1],
			GradeID:  row[2],
			LegalSex: db.LegalSex(row[3]),
		}
	}

	// An import accepts nothing: a roster change that would break an
	// existing placement is reported so an administrator can decide
	// deliberately, through the students endpoint, rather than being
	// waved through by the act of uploading a file.
	if err := app.upsertStudents(r, students, nil); err != nil {
		app.apiDBError(r, w, err,
			slog.String("admin_username", aui.Username), slog.Int("row_count", len(students)))

		return
	}

	app.logInfo(r, logMsgAdminStudentsImport,
		slog.String("admin_username", aui.Username), slog.Int("row_count", len(students)))
	app.wsHub.Broadcast(WSMessage("invalidate_students"))
	app.wsHub.Broadcast(WSMessage("invalidate_enrollments"))
	w.WriteHeader(http.StatusNoContent)
}
