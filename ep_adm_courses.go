package main

import (
	"bufio"
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

type adminCourse struct {
	Course               db.GetCoursesRow
	AllowedLegalSexes    []db.LegalSex
	AllowedLegalSexesMap map[db.LegalSex]bool
	AllowedGrades        []string
	AllowedGradesMap     map[string]bool
}

func (app *App) handleAdmCourses(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCourses", slog.String("admin_username", aui.Username))
	if r.Method != http.MethodGet {
		app.apiError(r, w, http.StatusMethodNotAllowed, nil, slog.String("admin_username", aui.Username))
		return
	}

	courses, err := app.queries.GetCourses(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	grades, err := app.queries.GetGrades(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	legalSexRestrictions, err := app.queries.GetCourseAllowedLegalSexes(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	gradeRestrictions, err := app.queries.GetCourseAllowedGrades(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	courseViews := make([]adminCourse, len(courses))
	courseByID := make(map[string]*adminCourse, len(courses))
	for i := range courses {
		courseViews[i] = adminCourse{
			Course: courses[i],
		}
		courseByID[courses[i].ID] = &courseViews[i]
	}

	for _, restriction := range legalSexRestrictions {
		if course, ok := courseByID[restriction.CourseID]; ok {
			course.AllowedLegalSexes = append(course.AllowedLegalSexes, restriction.LegalSex)
			if course.AllowedLegalSexesMap == nil {
				course.AllowedLegalSexesMap = make(map[db.LegalSex]bool)
			}
			course.AllowedLegalSexesMap[restriction.LegalSex] = true
		}
	}

	for _, restriction := range gradeRestrictions {
		if course, ok := courseByID[restriction.CourseID]; ok {
			course.AllowedGrades = append(course.AllowedGrades, restriction.Grade)
			if course.AllowedGradesMap == nil {
				course.AllowedGradesMap = make(map[string]bool)
			}
			course.AllowedGradesMap[restriction.Grade] = true
		}
	}

	for i := range courseViews {
		if len(courseViews[i].AllowedLegalSexes) > 1 {
			sort.Slice(courseViews[i].AllowedLegalSexes, func(a, b int) bool {
				return courseViews[i].AllowedLegalSexes[a] < courseViews[i].AllowedLegalSexes[b]
			})
		}
		if len(courseViews[i].AllowedGrades) > 1 {
			sort.Strings(courseViews[i].AllowedGrades)
		}
	}

	if err := app.admRenderTemplate(w, r, "courses", struct {
		Courses     []adminCourse
		Categories  []string
		Periods     []string
		Grades      []db.Grade
		Memberships []db.MembershipType
		LegalSexes  []db.LegalSex
	}{
		Courses:     courseViews,
		Categories:  categories,
		Periods:     periods,
		Grades:      grades,
		Memberships: []db.MembershipType{db.MembershipTypeFree, db.MembershipTypeInviteOnly},
		LegalSexes:  []db.LegalSex{db.LegalSexF, db.LegalSexM, db.LegalSexX},
	}, slog.String("admin_username", aui.Username)); err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\nfailed rendering template", err, slog.String("admin_username", aui.Username))
	}
}

func (app *App) handleAdmCoursesNew(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesNew", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course with an empty name, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course without a period, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	maxStudentsStr := strings.TrimSpace(r.FormValue("max_students"))
	maxStudents, err := strconv.ParseInt(maxStudentsStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMax students must be a number", err, slog.String("admin_username", aui.Username))
		return
	}
	if maxStudents < 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMax students cannot be negative", nil, slog.String("admin_username", aui.Username))
		return
	}

	membership := db.MembershipType(strings.TrimSpace(r.FormValue("membership")))
	switch membership {
	case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown membership type", nil, slog.String("admin_username", aui.Username))
		return
	}

	teacher := strings.TrimSpace(r.FormValue("teacher"))
	if teacher == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course without a teacher, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course without a location, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to add a course without a category, which is not allowed", nil, slog.String("admin_username", aui.Username))
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
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex value", nil, slog.String("admin_username", aui.Username))
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
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown grade value", nil, slog.String("admin_username", aui.Username))
			return
		}
		if _, ok := gradeSeen[grade]; ok {
			continue
		}
		gradeSeen[grade] = struct{}{}
		allowedGrades = append(allowedGrades, grade)
	}

	// TODO: transactions!!!

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
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	for _, ls := range legalSexes {
		err = app.queries.AddCourseAllowedLegalSex(r.Context(), db.AddCourseAllowedLegalSexParams{
			CourseID: id,
			LegalSex: ls,
		})
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
			return
		}
	}

	for _, grade := range allowedGrades {
		err = app.queries.AddCourseAllowedGrade(r.Context(), db.AddCourseAllowedGradeParams{
			CourseID: id,
			Grade:    grade,
		})
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
			return
		}
	}

	app.logInfo(r, "created course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_courses"})

	app.logInfo(r, "redirecting after new course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (app *App) handleAdmCoursesEdit(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesEdit", slog.String("admin_username", aui.Username))
	err := r.ParseForm()
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\n"+err.Error(), err, slog.String("admin_username", aui.Username))
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course with an empty name, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))

	period := strings.TrimSpace(r.FormValue("period"))
	if period == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course without a period, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	maxStudentsStr := strings.TrimSpace(r.FormValue("max_students"))
	maxStudents, err := strconv.ParseInt(maxStudentsStr, 10, 64)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMax students must be a number", err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}
	if maxStudents < 0 {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nMax students cannot be negative", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	membership := db.MembershipType(strings.TrimSpace(r.FormValue("membership")))
	switch membership {
	case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
	default:
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown membership type", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	teacher := strings.TrimSpace(r.FormValue("teacher"))
	if teacher == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course without a teacher, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course without a location, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to edit a course without a category, which is not allowed", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
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
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown legal sex value", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
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
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnknown grade value", nil, slog.String("admin_username", aui.Username), slog.String("course_id", id))
			return
		}
		if _, ok := gradeSeen[grade]; ok {
			continue
		}
		gradeSeen[grade] = struct{}{}
		allowedGrades = append(allowedGrades, grade)
	}

	tx, err := app.pool.Begin(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}
	defer tx.Rollback(r.Context())

	qtx := app.queries.WithTx(tx)

	err = qtx.UpdateCourse(r.Context(), db.UpdateCourseParams{
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
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	err = qtx.DeleteCourseAllowedLegalSexes(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	err = qtx.DeleteCourseAllowedGrades(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	for _, ls := range legalSexes {
		err = qtx.AddCourseAllowedLegalSex(r.Context(), db.AddCourseAllowedLegalSexParams{
			CourseID: id,
			LegalSex: ls,
		})
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
			return
		}
	}

	for _, grade := range allowedGrades {
		err = qtx.AddCourseAllowedGrade(r.Context(), db.AddCourseAllowedGradeParams{
			CourseID: id,
			Grade:    grade,
		})
		if err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
			return
		}
	}

	err = tx.Commit(r.Context())
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	app.logInfo(r, "updated course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_courses"})

	app.logInfo(r, "redirecting after edit course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

func (app *App) handleAdmCoursesDelete(w http.ResponseWriter, r *http.Request, aui *UserInfoAdmin) {
	app.logRequestStart(r, "handleAdmCoursesDelete", slog.String("admin_username", aui.Username))
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nYou are trying to delete a course with an empty ID, which is not allowed", nil, slog.String("admin_username", aui.Username))
		return
	}

	err := app.queries.DeleteCourse(r.Context(), id)
	if err != nil {
		app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.String("course_id", id))
		return
	}

	app.logInfo(r, "deleted course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_courses"})

	app.logInfo(r, "redirecting after delete course", slog.String("admin_username", aui.Username), slog.String("course_id", id))
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}

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

	f, _, err := r.FormFile("csv")
	if err != nil {
		app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nCSV file required", err, slog.String("admin_username", aui.Username))
		return
	}
	defer f.Close()

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

	expected := []string{
		"id",
		"name",
		"description",
		"period",
		"max_students",
		"membership",
		"teacher",
		"location",
		"category",
		"allowed_legal_sexes",
		"allowed_grades",
	}
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
	defer tx.Rollback(r.Context())

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
		if len(record) != len(expected) {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nUnexpected column count in CSV row", nil, slog.String("admin_username", aui.Username), slog.Int("row", row))
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
		period := strings.TrimSpace(record[3])
		if period == "" {
			app.respondHTTPError(r, w, http.StatusBadRequest, "Bad Request\nRow has empty period", nil, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
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
			Period:      period,
			MaxStudents: maxStudents,
			Membership:  membership,
			Teacher:     teacher,
			Location:    location,
			CategoryID:  category,
		}); err != nil {
			app.respondHTTPError(r, w, http.StatusInternalServerError, "Internal Server Error\n"+err.Error(), err, slog.String("admin_username", aui.Username), slog.Int("row", row), slog.String("course_id", id))
			return
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

	app.logInfo(r, "imported courses", slog.String("admin_username", aui.Username))
	app.broker.Broadcast(BrokerMsg{event: "invalidate_courses"})

	app.logInfo(r, "redirecting after course import", slog.String("admin_username", aui.Username))
	http.Redirect(w, r, "/admin/courses", http.StatusSeeOther)
}
