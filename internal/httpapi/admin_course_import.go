package httpapi

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

func (app *App) handleAdmCoursesImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesImport", slog.String("admin_username", aui.Username))
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

	expected := []string{
		"id",
		"name",
		"description",
		"periods",
		"max_students",
		"membership",
		"teacher",
		"location",
		"category",
		"allowed_legal_sexes",
		"allowed_grades",
	}
	header = normalizeTabularRecord(format, header, len(expected))
	if err := validateTabularHeader(header, expected); err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	tx, err := app.pool.Begin(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = tx.Rollback(r.Context())
	}()

	qtx := app.queries.WithTx(tx)

	row := 2 // header is row 1
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

		id := strings.TrimSpace(record[0])
		if id == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty course ID", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		name := strings.TrimSpace(record[1])
		if name == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty course name", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}

		description := strings.TrimSpace(record[2])
		periodIDs := normalizeStringSet(strings.Split(record[3], ","))
		if len(periodIDs) == 0 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has no periods", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}

		maxStudents, err := strconv.ParseInt(strings.TrimSpace(record[4]), 10, 64)
		if err != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid max_students value", err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}
		if maxStudents < 0 {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nNegative max_students value", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}

		membership := db.MembershipType(strings.TrimSpace(record[5]))
		switch membership {
		case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
		default:
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown membership type "+record[5], nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}

		teacher := strings.TrimSpace(record[6])
		location := strings.TrimSpace(record[7])

		category := strings.TrimSpace(record[8])
		if category == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty category", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}

		legalSexField := strings.TrimSpace(record[9])
		var legalSexes []db.LegalSex
		if legalSexField != "" {
			for _, part := range strings.Split(legalSexField, ",") {
				ls := db.LegalSex(strings.TrimSpace(part))
				switch ls {
				case db.LegalSexF, db.LegalSexM, db.LegalSexX:
				default:
					app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex "+part, nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
					return
				}
				legalSexes = append(legalSexes, ls)
			}
		}

		gradeField := strings.TrimSpace(record[10])
		var allowedGrades []string
		if gradeField != "" {
			for _, part := range strings.Split(gradeField, ",") {
				grade := strings.TrimSpace(part)
				if grade == "" {
					app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid blank grade entry", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
					return
				}
				allowedGrades = append(allowedGrades, grade)
			}
		}

		if err = qtx.NewCourse(r.Context(), db.NewCourseParams{
			ID:          id,
			Name:        name,
			Description: description,
			MaxStudents: maxStudents,
			Membership:  membership,
			Teacher:     teacher,
			Location:    location,
			CategoryID:  category,
		}); err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error()+"\n"+fmt.Sprintf("%#v", record), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
		}
		for _, periodID := range periodIDs {
			if err = qtx.AddCoursePeriod(r.Context(), db.AddCoursePeriodParams{
				CourseID: id,
				PeriodID: periodID,
			}); err != nil {
				app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id), slog.String("period_id", periodID))
				return
			}
		}

		seenLegalSex := make(map[db.LegalSex]struct{})
		for _, ls := range legalSexes {
			if _, ok := seenLegalSex[ls]; ok {
				continue
			}
			seenLegalSex[ls] = struct{}{}
			if err = qtx.AddCourseAllowedLegalSex(r.Context(), db.AddCourseAllowedLegalSexParams{
				CourseID: id,
				LegalSex: ls,
			}); err != nil {
				app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
				return
			}
		}

		seenGrades := make(map[string]struct{})
		for _, grade := range allowedGrades {
			if _, ok := seenGrades[grade]; ok {
				continue
			}
			seenGrades[grade] = struct{}{}
			if err = qtx.AddCourseAllowedGrade(r.Context(), db.AddCourseAllowedGradeParams{
				CourseID: id,
				Grade:    grade,
			}); err != nil {
				app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
				return
			}
		}

		row++
	}

	if err = tx.Commit(r.Context()); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	app.logInfo(r, logMsgAdminCoursesImport, slog.String("admin_username", aui.Username), slog.String("format", string(format)))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func coursePeriodIDsFromForm(r *http.Request) []string {
	values := append([]string{}, r.PostForm["period_ids"]...)
	values = append(values, r.PostForm["periods"]...)
	if legacy := strings.TrimSpace(r.FormValue("period")); legacy != "" {
		values = append(values, legacy)
	}
	return normalizeStringSet(values)
}
