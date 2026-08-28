package web

import (
	"bytes"
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Exports are the inverse of the imports and share their columns, so
// a file that comes out can go back in. That round trip is what makes
// "export, edit in a spreadsheet, re-import" a supported way to work
// rather than a thing that happens to survive.

// writeCSV emits a spreadsheet and reports whether it got all the way
// out.
func (app *Server) writeCSV(w http.ResponseWriter, r *http.Request, filename string, records [][]string) bool {
	var buf bytes.Buffer

	// Excel reads a mark-less UTF-8 file as the local code page,
	// which turns every non-ASCII name into mojibake. The import side
	// strips this again.
	if _, err := buf.WriteString("\uFEFF"); err != nil {
		app.apiInternalError(r, w, err)

		return false
	}

	// Every cell, including the header, since the guard is the
	// import's inverse and has to be applied uniformly to be one.
	for _, record := range records {
		for i := range record {
			record[i] = csvEscapeCell(record[i])
		}
	}

	csvWriter := csv.NewWriter(&buf)

	if err := csvWriter.WriteAll(records); err != nil {
		app.apiInternalError(r, w, err)

		return false
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(buf.Bytes()); err != nil {
		app.logWarn(r, logMsgHTTPResponseError, slog.Any("error", err))

		return false
	}

	return true
}

func (app *Server) handleAdmStudentsExport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsExport", slog.String("admin_username", aui.Username))

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	students, err := app.queries.GetStudents(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	records := [][]string{studentImportColumns}
	for _, student := range students {
		records = append(records, []string{
			student.ID,
			student.Name,
			student.GradeID,
			string(student.LegalSex),
		})
	}

	if app.writeCSV(w, r, "students.csv", records) {
		app.logInfo(r, logMsgAdminStudentsExport,
			slog.String("admin_username", aui.Username), slog.Int("row_count", len(students)))
	}
}

// exportCount spells a capacity for the import to read back.
//
// No cap is NULL, and the word is written rather than the blank cell
// that also means it: a blank in a capacity column reads as an
// oversight, and this file is the one an administrator edits and
// re-uploads. The importer takes either.
func exportCount(v pgtype.Int8) string {
	if !v.Valid {
		return "unlimited"
	}

	return strconv.FormatInt(v.Int64, 10)
}

func (app *Server) handleAdmCoursesExport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesExport", slog.String("admin_username", aui.Username))

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	courses, err := app.queries.GetCourses(ctx)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("admin_username", aui.Username))

		return
	}

	records := [][]string{courseImportColumns}

	for _, course := range courses {
		legalSexes := make([]string, len(course.AllowedLegalSexes))
		for i, ls := range course.AllowedLegalSexes {
			legalSexes[i] = string(ls)
		}

		records = append(records, []string{
			course.ID,
			course.Name,
			course.Description,
			strings.Join(course.PeriodIds, ","),
			exportCount(course.MaxStudents),
			strconv.FormatBool(course.InviteOnly),
			course.Teacher,
			course.TeacherEmail,
			course.Location,
			course.Term,
			course.Cost,
			course.CategoryID,
			strings.Join(legalSexes, ","),
			strings.Join(course.AllowedGradeIds, ","),
		})
	}

	if app.writeCSV(w, r, "courses.csv", records) {
		app.logInfo(r, logMsgAdminCoursesExport,
			slog.String("admin_username", aui.Username), slog.Int("row_count", len(courses)))
	}
}
