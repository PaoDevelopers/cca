package main

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleAdmStudents(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	students, err := app.queries.GetStudents(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	grades, err := app.queries.GetGrades(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "students", struct {
		Students   []db.Student
		Grades     []db.Grade
		LegalSexes []db.LegalSex
	}{
		Students:   students,
		Grades:     grades,
		LegalSexes: []db.LegalSex{db.LegalSexF, db.LegalSexM, db.LegalSexX},
	})
}

func (app *App) handleAdmStudentsNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		http.Error(w, "Bad Request\nYou are trying to add a student with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Bad Request\nYou are trying to add a student with an empty name, which is not allowed", http.StatusBadRequest)
		return
	}

	grade := strings.TrimSpace(r.FormValue("grade"))
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to add a student without a grade, which is not allowed", http.StatusBadRequest)
		return
	}

	legalSex := db.LegalSex(strings.TrimSpace(r.FormValue("legal_sex")))
	switch legalSex {
	case db.LegalSexF, db.LegalSexM, db.LegalSexX:
	default:
		http.Error(w, "Bad Request\nUnknown legal sex value", http.StatusBadRequest)
		return
	}

	err = app.queries.NewStudent(r.Context(), db.NewStudentParams{
		ID:       id,
		Name:     name,
		Grade:    grade,
		LegalSex: legalSex,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a student with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a student with an empty name, which is not allowed", http.StatusBadRequest)
		return
	}

	grade := strings.TrimSpace(r.FormValue("grade"))
	if grade == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a student without a grade, which is not allowed", http.StatusBadRequest)
		return
	}

	legalSex := db.LegalSex(strings.TrimSpace(r.FormValue("legal_sex")))
	switch legalSex {
	case db.LegalSexF, db.LegalSexM, db.LegalSexX:
	default:
		http.Error(w, "Bad Request\nUnknown legal sex value", http.StatusBadRequest)
		return
	}

	err = app.queries.UpdateStudent(r.Context(), db.UpdateStudentParams{
		ID:       id,
		Name:     name,
		Grade:    grade,
		LegalSex: legalSex,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		http.Error(w, "Bad Request\nYou are trying to delete a student with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nStudent ID must be a number", http.StatusBadRequest)
		return
	}

	err = app.queries.DeleteStudent(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}

func (app *App) handleAdmStudentsImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	err := r.ParseMultipartForm(8 << 20)
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	f, _, err := r.FormFile("csv")
	if err != nil {
		http.Error(w, "Bad Request\nCSV file required", http.StatusBadRequest)
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			http.Error(w, "Bad Request\nEmpty CSV", http.StatusBadRequest)
			return
		}
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	expected := []string{"id", "name", "grade", "legal_sex"}
	if len(header) != len(expected) {
		http.Error(w, "Bad Request\nCSV header does not match expected column count", http.StatusBadRequest)
		return
	}
	for i, col := range header {
		if strings.TrimSpace(col) != expected[i] {
			http.Error(w, "Bad Request\nUnexpected header column: "+col, http.StatusBadRequest)
			return
		}
	}

	tx, err := app.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := app.queries.WithTx(tx)

	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
			return
		}
		if len(record) != len(expected) {
			http.Error(w, "Bad Request\nUnexpected column count in CSV row", http.StatusBadRequest)
			return
		}

		idStr := strings.TrimSpace(record[0])
		if idStr == "" {
			http.Error(w, "Bad Request\nRow has empty student ID", http.StatusBadRequest)
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "Bad Request\nInvalid student ID "+idStr, http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(record[1])
		if name == "" {
			http.Error(w, "Bad Request\nRow has empty student name", http.StatusBadRequest)
			return
		}

		grade := strings.TrimSpace(record[2])
		if grade == "" {
			http.Error(w, "Bad Request\nRow has empty grade", http.StatusBadRequest)
			return
		}

		legalSex := db.LegalSex(strings.TrimSpace(record[3]))
		switch legalSex {
		case db.LegalSexF, db.LegalSexM, db.LegalSexX:
		default:
			http.Error(w, "Bad Request\nUnknown legal sex "+record[3], http.StatusBadRequest)
			return
		}

		err = qtx.NewStudent(r.Context(), db.NewStudentParams{
			ID:       id,
			Name:     name,
			Grade:    grade,
			LegalSex: legalSex,
		})
		if err != nil {
			http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err = tx.Commit(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/students", http.StatusSeeOther)
}
