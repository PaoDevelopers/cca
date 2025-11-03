package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleAdmStudents(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudents", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	students, err := app.queries.GetStudents(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	grades, err := app.queries.GetGrades(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	if err := app.admRenderTemplate(w, r, "students", struct {
		Students   []db.Student
		Grades     []db.Grade
		LegalSexes []db.LegalSex
	}{
		Students:   students,
		Grades:     grades,
		LegalSexes: []db.LegalSex{db.LegalSexF, db.LegalSexM, db.LegalSexX},
	}, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmStudentsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsNew", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a student with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a student with an empty name, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	grade := strings.TrimSpace(r.FormValue("grade"))
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a student without a grade, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	legalSex := db.LegalSex(strings.TrimSpace(r.FormValue("legal_sex")))
	switch legalSex {
	case db.LegalSexF, db.LegalSexM, db.LegalSexX:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex value", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	if err = app.queries.NewStudent(r.Context(), db.NewStudentParams{
		ID:       id,
		Name:     name,
		Grade:    grade,
		LegalSex: legalSex,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	app.logInfo(r, logMsgAdminStudentsCreate, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsEdit", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a student with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a student with an empty name, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	grade := strings.TrimSpace(r.FormValue("grade"))
	if grade == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a student without a grade, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	legalSex := db.LegalSex(strings.TrimSpace(r.FormValue("legal_sex")))
	switch legalSex {
	case db.LegalSexF, db.LegalSexM, db.LegalSexX:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex value", nil, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	if err = app.queries.UpdateStudent(r.Context(), db.UpdateStudentParams{
		ID:       id,
		Name:     name,
		Grade:    grade,
		LegalSex: legalSex,
	}); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	app.logInfo(r, logMsgAdminStudentsUpdate, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsDelete", slog.String("admin_username", aui.Username))
	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a student with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nStudent ID must be a number", err, slog.String("admin_username", aui.Username))
		return
	}

	if err = app.queries.DeleteStudent(r.Context(), id); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
		return
	}

	app.logInfo(r, logMsgAdminStudentsDelete, slog.String("admin_username", aui.Username), slog.Int64("student_id", id))
	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmStudentsImport", slog.String("admin_username", aui.Username))
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

	expected := []string{"id", "name", "grade", "legal_sex"}
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

	tx, err := app.pool.Begin(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}
	defer func() {
		_ = tx.Rollback(r.Context())
	}()

	qtx := app.queries.WithTx(tx)

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

		idStr := strings.TrimSpace(record[0])
		if idStr == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty student ID", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nInvalid student ID "+idStr, err, slog.String("admin_username", aui.Username), slog.Int("row", row))
			return
		}

		name := strings.TrimSpace(record[1])
		if name == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty student name", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.Int64("student_id", id))
			return
		}

		grade := strings.TrimSpace(record[2])
		if grade == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty grade", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.Int64("student_id", id))
			return
		}

		legalSex := db.LegalSex(strings.TrimSpace(record[3]))
		switch legalSex {
		case db.LegalSexF, db.LegalSexM, db.LegalSexX:
		default:
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex "+record[3], nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.Int64("student_id", id))
			return
		}

		if err = qtx.NewStudent(r.Context(), db.NewStudentParams{
			ID:       id,
			Name:     name,
			Grade:    grade,
			LegalSex: legalSex,
		}); err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.Int64("student_id", id))
			return
		}

		row++
	}

	if err := tx.Commit(r.Context()); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	app.logInfo(r, logMsgAdminStudentsImport, slog.String("admin_username", aui.Username))
	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}
