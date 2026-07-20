package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

// TODO: See how SSEs should be handled here. We may need a way to map from usernames to connections.
// Not using SSE anymore

func (app *App) handleAdmSelections(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelections", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	selections, err := app.queries.GetSelections(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	students, err := app.queries.GetStudents(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "selections", struct {
		Selections     []db.GetSelectionsRow
		Students       []db.Student
		Courses        []db.GetCoursesRow
		SelectionTypes []db.SelectionType
	}{
		Selections:     selections,
		Students:       students,
		Courses:        courses,
		SelectionTypes: []db.SelectionType{db.SelectionTypeNormal, db.SelectionTypeInvite, db.SelectionTypeForce},
	}, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmSelectionsExport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsExport", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	rows, err := app.queries.GetSelectionsExport(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	var buf bytes.Buffer
	if _, err := buf.WriteString("\uFEFF"); err != nil { // Excel BOM
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}
	csvWriter := csv.NewWriter(&buf)
	if err := csvWriter.Write([]string{"student_id", "student_name", "grade", "legal_sex", "course_id", "course_name", "periods", "selection_type"}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	for _, row := range rows {
		record := []string{
			strconv.FormatInt(row.StudentID, 10),
			row.StudentName,
			row.Grade,
			string(row.LegalSex),
			row.CourseID,
			row.CourseName,
			row.Periods,
			string(row.SelectionType),
		}
		if err := csvWriter.Write(record); err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
			return
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=\"selections.csv\"")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		app.logWarn(r, logMsgHTTPResponseError, slog.Any("error", err), slog.String("admin_username", aui.Username))
	}
	app.logInfo(r, logMsgAdminSelectionsExport, slog.String("admin_username", aui.Username), slog.Int("row_count", len(rows)))
}

func (app *App) handleAdmSelectionsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsNew", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	rawStudentIDs := r.PostForm["student_ids"]
	if len(rawStudentIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nSelect at least one student", nil, slog.String("admin_username", aui.Username))
		return
	}

	var studentIDs []int64
	studentSeen := make(map[int64]struct{}, len(rawStudentIDs))
	for _, raw := range rawStudentIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", parseErr, slog.String("admin_username", aui.Username))
			return
		}
		if _, ok := studentSeen[id]; ok {
			continue
		}
		studentSeen[id] = struct{}{}
		studentIDs = append(studentIDs, id)
	}
	if len(studentIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nNo valid student IDs provided", nil, slog.String("admin_username", aui.Username))
		return
	}

	rawCourseIDs := r.PostForm["course_ids"]
	if len(rawCourseIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nSelect at least one course", nil, slog.String("admin_username", aui.Username))
		return
	}

	var courseIDs []string
	courseSeen := make(map[string]struct{}, len(rawCourseIDs))
	for _, raw := range rawCourseIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := courseSeen[id]; ok {
			continue
		}
		courseSeen[id] = struct{}{}
		courseIDs = append(courseIDs, id)
	}
	if len(courseIDs) == 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nNo valid course IDs provided", nil, slog.String("admin_username", aui.Username))
		return
	}
	periodIDs := make([]string, 0, len(courseIDs))
	for _, courseID := range courseIDs {
		coursePeriods, queryErr := app.queries.GetCoursePeriodsByCourse(r.Context(), courseID)
		if queryErr != nil || len(coursePeriods) != 1 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nChoose a specific timetable slot in the React administrator app", queryErr, slog.String("admin_username", aui.Username), slog.String("course_id", courseID))
			return
		}
		periodIDs = append(periodIDs, coursePeriods[0])
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNormal, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown selection type", nil, slog.String("admin_username", aui.Username))
		return
	}

	batch := cartesianSelectionBatch(studentIDs, courseIDs, periodIDs, selectionType)
	if _, err = app.queries.NewSelectionsBatch(r.Context(), batch); err != nil {
		app.respondHTTPError(
			r,
			w,
			http.StatusInternalServerError,
			"Internal Server Error\n"+err.Error(),
			err,
			slog.String("admin_username", aui.Username),
		)
		return
	}

	app.logInfo(
		r,
		logMsgAdminSelectionsCreate,
		slog.String("admin_username", aui.Username),
		slog.Any("student_ids", studentIDs),
		slog.Any("course_ids", courseIDs),
		slog.String("selection_type", string(selectionType)),
	)
	app.wsHub.BroadcastToStudents(studentIDs, WSMessage("invalidate_selections"))
	app.broadcastCourseCounts(r, courseIDs)
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsEdit", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without a student ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	currentCourseID := strings.TrimSpace(r.FormValue("current_course_id"))
	if currentCourseID == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without its current course ID", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}

	courseID := strings.TrimSpace(r.FormValue("course_id"))
	if courseID == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a selection without a course ID, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}
	periodID := strings.TrimSpace(r.FormValue("period_id"))
	if periodID == "" {
		coursePeriods, queryErr := app.queries.GetCoursePeriodsByCourse(r.Context(), courseID)
		if queryErr != nil || len(coursePeriods) != 1 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nChoose a specific timetable slot in the React administrator app", queryErr, slog.String("admin_username", aui.Username), slog.String("course_id", courseID))
			return
		}
		periodID = coursePeriods[0]
	}

	selectionType := db.SelectionType(strings.TrimSpace(r.FormValue("selection_type")))
	switch selectionType {
	case db.SelectionTypeNormal, db.SelectionTypeInvite, db.SelectionTypeForce:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown selection type", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID))
		return
	}

	if _, err = app.queries.UpdateSelection(r.Context(), db.UpdateSelectionParams{
		StudentID:       studentID,
		CurrentCourseID: currentCourseID,
		NewCourseID:     courseID,
		NewPeriodID:     periodID,
		SelectionType:   selectionType,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID), slog.String("current_course_id", currentCourseID))
		return
	}

	app.logInfo(r, logMsgAdminSelectionsUpdate, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID), slog.String("current_course_id", currentCourseID), slog.String("selection_type", string(selectionType)))
	app.wsHub.BroadcastToStudents([]int64{studentID}, WSMessage("invalidate_selections"))
	courseSet := []string{courseID}
	if currentCourseID != courseID {
		courseSet = append(courseSet, currentCourseID)
	}
	app.broadcastCourseCounts(r, courseSet)
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

func (app *App) handleAdmSelectionsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmSelectionsDelete", slog.String("admin_username", aui.Username))
	studentIDStr := strings.TrimSpace(r.FormValue("student_id"))
	if studentIDStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a selection without a student ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	studentID, err := strconv.ParseInt(studentIDStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	courseID := strings.TrimSpace(r.FormValue("course_id"))
	if courseID == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a selection without a course ID", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID))
		return
	}

	if err = app.queries.DeleteSelection(r.Context(), db.DeleteSelectionParams{
		StudentID: studentID,
		CourseID:  courseID,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID))
		return
	}

	app.logInfo(r, logMsgAdminSelectionsDelete, slog.String("admin_username", aui.Username), slog.Int64("student_id", studentID), slog.String("course_id", courseID))
	app.wsHub.BroadcastToStudents([]int64{studentID}, WSMessage("invalidate_selections"))
	app.broadcastCourseCounts(r, []string{courseID})
	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}

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

	f, _, err := r.FormFile("csv")
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nCSV file required", err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = f.Close()
	}()

	br := bufio.NewReader(f)
	if b, _ := br.Peek(3); len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		if _, err := br.Discard(3); err != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
			return
		}
	}

	reader := csv.NewReader(br)
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nEmpty CSV", err, slog.String("admin_username", aui.Username))
			return
		}
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	expected := []string{"course_id", "period_id", "student_id", "selection_type"}
	if len(header) != len(expected) {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nCSV header does not match expected column count", nil, slog.String("admin_username", aui.Username))
		return
	}
	for i, col := range header {
		if strings.TrimSpace(col) != expected[i] {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnexpected header column: "+col, nil, slog.String("admin_username", aui.Username))
			return
		}
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
		if len(record) != len(expected) {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnexpected column count in CSV row", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
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
	app.logInfo(r, logMsgAdminSelectionsImport, slog.String("admin_username", aui.Username), slog.Int("rows", len(records)), slog.Int("students_impacted", len(students)), slog.Int("courses_impacted", len(courses)))
	if len(students) > 0 {
		app.wsHub.BroadcastToStudents(students, WSMessage("invalidate_selections"))
	}
	app.broadcastCourseCounts(r, courses)

	http.Redirect(w, r, "/admin/selections", http.StatusSeeOther)
}
