package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	db "git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

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

	expected := []string{"id", "name", "grade", "legal_sex"}
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

	app.logInfo(r, logMsgAdminStudentsImport, slog.String("admin_username", aui.Username), slog.String("format", string(format)))
	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}
