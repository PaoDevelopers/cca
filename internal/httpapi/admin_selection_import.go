package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

func (app *App) handleAdmSelectionsImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsImport", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodPost {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	format, f, _, err := openTabularUpload(r)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = f.Close()
	}()

	reader, err := newTabularRowReader(format, f)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = reader.Close()
	}()
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nEmpty data file", err, slog.String("admin_username", aui.Username))
			return
		}
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	expected := []string{"course_id", "period_id", "student_id", "selection_type"}
	header = normalizeTabularRecord(format, header, len(expected))
	if err := validateTabularHeader(header, expected); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	type importedSelection struct {
		studentID     int64
		courseID      string
		periodID      string
		selectionType db.SelectionType
	}
	records := make([]importedSelection, 0)
	seen := make(map[struct {
		studentID int64
		courseID  string
	}]struct{})

	row := 2
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}
		record = normalizeTabularRecord(format, record, len(expected))
		if len(record) != len(expected) {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnexpected column count in row", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		courseID := strings.TrimSpace(record[0])
		if courseID == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty course ID", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		periodID := strings.TrimSpace(record[1])
		if periodID == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty period ID", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		studentIDStr := strings.TrimSpace(record[2])
		studentID, parseErr := strconv.ParseInt(studentIDStr, 10, 64)
		if parseErr != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid student ID "+studentIDStr, parseErr, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		selectionTypeStr := strings.ToLower(strings.TrimSpace(record[3]))
		var selectionType db.SelectionType
		switch selectionTypeStr {
		case string(db.SelectionTypeNormal):
			selectionType = db.SelectionTypeNormal
		case string(db.SelectionTypeInvite):
			selectionType = db.SelectionTypeInvite
		case string(db.SelectionTypeForce):
			selectionType = db.SelectionTypeForce
		default:
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown selection type", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("selection_type", selectionTypeStr))
			return
		}

		key := struct {
			studentID int64
			courseID  string
		}{studentID: studentID, courseID: courseID}
		if _, duplicate := seen[key]; duplicate {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nDuplicate student/course pair in CSV", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", courseID), slog.Int64("student_id", studentID))
			return
		}
		seen[key] = struct{}{}
		records = append(records, importedSelection{
			studentID: studentID, courseID: courseID, periodID: periodID, selectionType: selectionType,
		})

		row++
	}

	studentSet := make(map[int64]struct{})
	courseSet := make(map[string]struct{})
	studentIDs := make([]int64, 0, len(records))
	courseIDs := make([]string, 0, len(records))
	periodIDs := make([]string, 0, len(records))
	selectionTypes := make([]db.SelectionType, 0, len(records))
	for _, record := range records {
		studentIDs = append(studentIDs, record.studentID)
		courseIDs = append(courseIDs, record.courseID)
		periodIDs = append(periodIDs, record.periodID)
		selectionTypes = append(selectionTypes, record.selectionType)
		studentSet[record.studentID] = struct{}{}
		courseSet[record.courseID] = struct{}{}
	}

	if _, err := app.queries.NewSelectionsBatch(r.Context(), db.NewSelectionsBatchParams{
		StudentIds: studentIDs, CourseIds: courseIDs, PeriodIds: periodIDs, SelectionTypes: selectionTypes,
	}); err != nil {
		app.writeClassifiedAPIError(r, w, err, slog.String("admin_username", aui.Username))
		return
	}

	students := make([]int64, 0, len(studentSet))
	for id := range studentSet {
		students = append(students, id)
	}
	sort.Slice(students, func(i, j int) bool { return students[i] < students[j] })
	courses := make([]string, 0, len(courseSet))
	for id := range courseSet {
		courses = append(courses, id)
	}
	sort.Strings(courses)
	app.logInfo(r, logMsgAdminSelectionsImport, slog.String("admin_username", aui.Username), slog.Int("rows", len(records)), slog.Int("students_impacted", len(students)), slog.Int("courses_impacted", len(courses)), slog.String("format", string(format)))
	if len(students) > 0 {
		app.wsHub.BroadcastToStudents(students, WSMessage("invalidate_selections"))
	}
	app.publishCourseStates(r, courses)

	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}
