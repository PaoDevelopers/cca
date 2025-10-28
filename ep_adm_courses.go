package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
)

func (app *App) handleAdmCourses(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusMethodNotAllowed, nil)
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	grades, err := app.queries.GetGrades(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	app.admRenderTemplate(w, "courses", struct {
		Courses     []db.Course
		Categories  []string
		Periods     []string
		Grades      []db.Grade
		Memberships []db.MembershipType
		LegalSexes  []db.LegalSex
	}{
		Courses:     courses,
		Categories:  categories,
		Periods:     periods,
		Grades:      grades,
		Memberships: []db.MembershipType{db.MembershipTypeFree, db.MembershipTypeInviteOnly},
		LegalSexes:  []db.LegalSex{db.LegalSexF, db.LegalSexM, db.LegalSexX},
	})
}

func (app *App) handleAdmCoursesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course with an empty name, which is not allowed", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course without a period, which is not allowed", http.StatusBadRequest)
		return
	}

	maxStudentsStr := strings.TrimSpace(r.FormValue("max_students"))
	maxStudents, err := strconv.ParseInt(maxStudentsStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nMax students must be a number", http.StatusBadRequest)
		return
	}
	if maxStudents < 0 {
		http.Error(w, "Bad Request\nMax students cannot be negative", http.StatusBadRequest)
		return
	}

	membership := db.MembershipType(strings.TrimSpace(r.FormValue("membership")))
	switch membership {
	case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
	default:
		http.Error(w, "Bad Request\nUnknown membership type", http.StatusBadRequest)
		return
	}

	teacher := strings.TrimSpace(r.FormValue("teacher"))
	if teacher == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course without a teacher, which is not allowed", http.StatusBadRequest)
		return
	}

	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course without a location, which is not allowed", http.StatusBadRequest)
		return
	}

	category := strings.TrimSpace(r.FormValue("category_id"))
	if category == "" {
		http.Error(w, "Bad Request\nYou are trying to add a course without a category, which is not allowed", http.StatusBadRequest)
		return
	}

	legalSexValues := r.PostForm["legal_sexes"]
	legalSexSeen := make(map[db.LegalSex]struct{})
	var legalSexes []db.LegalSex
	for _, value := range legalSexValues {
		ls := db.LegalSex(strings.TrimSpace(value))
		switch ls {
		case db.LegalSexF, db.LegalSexM, db.LegalSexX:
		default:
			http.Error(w, "Bad Request\nUnknown legal sex value", http.StatusBadRequest)
			return
		}
		if _, ok := legalSexSeen[ls]; ok {
			continue
		}
		legalSexSeen[ls] = struct{}{}
		legalSexes = append(legalSexes, ls)
	}

	gradeValues := r.PostForm["allowed_grades"]
	gradeSeen := make(map[string]struct{})
	var allowedGrades []string
	for _, value := range gradeValues {
		grade := strings.TrimSpace(value)
		if grade == "" {
			http.Error(w, "Bad Request\nUnknown grade value", http.StatusBadRequest)
			return
		}
		if _, ok := gradeSeen[grade]; ok {
			continue
		}
		gradeSeen[grade] = struct{}{}
		allowedGrades = append(allowedGrades, grade)
	}

	err = app.queries.NewCourse(r.Context(), db.NewCourseParams{
		ID:          id,
		Name:        name,
		Description: description,
		Period:      period,
		MaxStudents: maxStudents,
		Membership:  membership,
		Teacher:     teacher,
		Location:    location,
		CategoryID:  category,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, ls := range legalSexes {
		err = app.queries.AddCourseAllowedLegalSex(r.Context(), db.AddCourseAllowedLegalSexParams{
			CourseID: id,
			LegalSex: ls,
		})
		if err != nil {
			http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	for _, grade := range allowedGrades {
		err = app.queries.AddCourseAllowedGrade(r.Context(), db.AddCourseAllowedGradeParams{
			CourseID: id,
			Grade:    grade,
		})
		if err != nil {
			http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (app *App) handleAdmCoursesEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course with an empty name, which is not allowed", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course without a period, which is not allowed", http.StatusBadRequest)
		return
	}

	maxStudentsStr := strings.TrimSpace(r.FormValue("max_students"))
	maxStudents, err := strconv.ParseInt(maxStudentsStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request\nMax students must be a number", http.StatusBadRequest)
		return
	}
	if maxStudents < 0 {
		http.Error(w, "Bad Request\nMax students cannot be negative", http.StatusBadRequest)
		return
	}

	membership := db.MembershipType(strings.TrimSpace(r.FormValue("membership")))
	switch membership {
	case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
	default:
		http.Error(w, "Bad Request\nUnknown membership type", http.StatusBadRequest)
		return
	}

	teacher := strings.TrimSpace(r.FormValue("teacher"))
	if teacher == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course without a teacher, which is not allowed", http.StatusBadRequest)
		return
	}

	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course without a location, which is not allowed", http.StatusBadRequest)
		return
	}

	category := strings.TrimSpace(r.FormValue("category_id"))
	if category == "" {
		http.Error(w, "Bad Request\nYou are trying to edit a course without a category, which is not allowed", http.StatusBadRequest)
		return
	}

	err = app.queries.UpdateCourse(r.Context(), db.UpdateCourseParams{
		ID:          id,
		Name:        name,
		Description: description,
		Period:      period,
		MaxStudents: maxStudents,
		Membership:  membership,
		Teacher:     teacher,
		Location:    location,
		CategoryID:  category,
	})
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (app *App) handleAdmCoursesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Error(w, "Bad Request\nYou are trying to delete a course with an empty ID, which is not allowed", http.StatusBadRequest)
		return
	}

	err := app.queries.DeleteCourse(r.Context(), id)
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (app *App) handleAdmCoursesImport(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
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

	br := bufio.NewReader(f)
	if b, _ := br.Peek(3); len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		if _, err := br.Discard(3); err != nil {
			http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
			return
		}
	}

	reader := csv.NewReader(br)
	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			http.Error(w, "Bad Request\nEmpty CSV", http.StatusBadRequest)
			return
		}
		http.Error(w, "Bad Request\n"+err.Error(), http.StatusBadRequest)
		return
	}

	expected := []string{
		"id",
		"name",
		"description",
		"period",
		"max_students",
		"membership",
		"teacher",
		"location",
		"category_id",
		"allowed_legal_sexes",
		"allowed_grades",
	}
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

		id := strings.TrimSpace(record[0])
		if id == "" {
			http.Error(w, "Bad Request\nRow has empty course ID", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(record[1])
		if name == "" {
			http.Error(w, "Bad Request\nRow has empty course name", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(record[2])
		period := strings.TrimSpace(record[3])
		if period == "" {
			http.Error(w, "Bad Request\nRow has empty period", http.StatusBadRequest)
			return
		}

		maxStudents, err := strconv.ParseInt(strings.TrimSpace(record[4]), 10, 64)
		if err != nil {
			http.Error(w, "Bad Request\nInvalid max_students value", http.StatusBadRequest)
			return
		}
		if maxStudents < 0 {
			http.Error(w, "Bad Request\nNegative max_students value", http.StatusBadRequest)
			return
		}

		membership := db.MembershipType(strings.TrimSpace(record[5]))
		switch membership {
		case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
		default:
			http.Error(w, "Bad Request\nUnknown membership type "+record[5], http.StatusBadRequest)
			return
		}

		teacher := strings.TrimSpace(record[6])
		location := strings.TrimSpace(record[7])

		category := strings.TrimSpace(record[8])
		if category == "" {
			http.Error(w, "Bad Request\nRow has empty category", http.StatusBadRequest)
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
					http.Error(w, "Bad Request\nUnknown legal sex "+part, http.StatusBadRequest)
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
					http.Error(w, "Bad Request\nInvalid blank grade entry", http.StatusBadRequest)
					return
				}
				allowedGrades = append(allowedGrades, grade)
			}
		}

		err = qtx.NewCourse(r.Context(), db.NewCourseParams{
			ID:          id,
			Name:        name,
			Description: description,
			Period:      period,
			MaxStudents: maxStudents,
			Membership:  membership,
			Teacher:     teacher,
			Location:    location,
			CategoryID:  category,
		})
		if err != nil {
			http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
			return
		}

		seenLegalSex := make(map[db.LegalSex]struct{})
		for _, ls := range legalSexes {
			if _, ok := seenLegalSex[ls]; ok {
				continue
			}
			seenLegalSex[ls] = struct{}{}
			err = qtx.AddCourseAllowedLegalSex(r.Context(), db.AddCourseAllowedLegalSexParams{
				CourseID: id,
				LegalSex: ls,
			})
			if err != nil {
				http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		seenGrades := make(map[string]struct{})
		for _, grade := range allowedGrades {
			if _, ok := seenGrades[grade]; ok {
				continue
			}
			seenGrades[grade] = struct{}{}
			err = qtx.AddCourseAllowedGrade(r.Context(), db.AddCourseAllowedGradeParams{
				CourseID: id,
				Grade:    grade,
			})
			if err != nil {
				http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	err = tx.Commit(r.Context())
	if err != nil {
		http.Error(w, "Internal Server Error\n"+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}
