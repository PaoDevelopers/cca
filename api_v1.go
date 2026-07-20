package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/jackc/pgx/v5"
)

type sessionView struct {
	Role    string            `json:"role"`
	Admin   *adminSessionView `json:"admin,omitempty"`
	Student *UserInfoStudent  `json:"student,omitempty"`
}

type adminSessionView struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type studentBootstrapView struct {
	Session      sessionView                           `json:"session"`
	Courses      []CourseView                          `json:"courses"`
	Requirements []db.GetStudentRequirementProgressRow `json:"requirements"`
}

func (app *App) loadStudentBootstrap(ctx context.Context, student *UserInfoStudent) (studentBootstrapView, error) {
	view := studentBootstrapView{
		Session: sessionView{Role: "student", Student: student},
	}

	// Both SQL read models share one repeatable-read snapshot, so a response
	// cannot combine catalogue state from before a selection with requirement
	// progress from after it.
	tx, err := app.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return view, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := app.queries.WithTx(tx)

	view.Courses, err = listStudentCourseViewsWithQueries(ctx, queries, student)
	if err != nil {
		return view, err
	}
	view.Requirements, err = queries.GetStudentRequirementProgress(ctx, student.ID)
	if err != nil {
		return view, err
	}
	if view.Courses == nil {
		view.Courses = []CourseView{}
	}
	if view.Requirements == nil {
		view.Requirements = []db.GetStudentRequirementProgressRow{}
	}
	if err := tx.Commit(ctx); err != nil {
		return view, err
	}
	return view, nil
}

func (app *App) handleAPIStudentBootstrap(w http.ResponseWriter, r *http.Request, student *UserInfoStudent) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	view, err := app.loadStudentBootstrap(r.Context(), student)
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.writeJSON(r, w, http.StatusOK, view)
}

func (app *App) handleAPISession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	ui, err := app.authenticateRequest(r)
	if err != nil || ui == nil {
		app.writeAPIError(r, w, http.StatusUnauthorized, "unauthenticated", "Please sign in again.", err)
		return
	}
	switch user := ui.(type) {
	case *UserInfoStudent:
		app.writeJSON(r, w, http.StatusOK, sessionView{Role: "student", Student: user})
	case *UserInfoAdmin:
		app.writeJSON(r, w, http.StatusOK, sessionView{Role: "admin", Admin: &adminSessionView{
			ID:       user.ID,
			Username: user.Username,
		}})
	default:
		app.writeAPIError(r, w, http.StatusUnauthorized, "unauthenticated", "Please sign in again.", nil)
	}
}

func (app *App) handleAPIStudentCourses(w http.ResponseWriter, r *http.Request, student *UserInfoStudent) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	courses, err := app.listStudentCourseViews(r.Context(), student)
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.writeJSON(r, w, http.StatusOK, courses)
}

func (app *App) handleAPIStudentPeriods(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	if periods == nil {
		periods = []string{}
	}
	app.writeJSON(r, w, http.StatusOK, periods)
}

func (app *App) handleAPIStudentGrades(w http.ResponseWriter, r *http.Request, _ *UserInfoStudent) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	grades, err := app.AbsGrades(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.writeJSON(r, w, http.StatusOK, grades)
}

type selectionMutation struct {
	CourseID string `json:"course_id"`
	PeriodID string `json:"period_id"`
}

func (app *App) handleAPIStudentSelections(w http.ResponseWriter, r *http.Request, student *UserInfoStudent) {
	switch r.Method {
	case http.MethodGet:
		selections, err := app.queries.GetSelectionsByStudent(r.Context(), student.ID)
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if selections == nil {
			selections = []db.GetSelectionsByStudentRow{}
		}
		app.writeJSON(r, w, http.StatusOK, selections)
	case http.MethodPost, http.MethodDelete:
		var mutation selectionMutation
		if err := decodeAPIJSON(w, r, &mutation); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		mutation.CourseID = strings.TrimSpace(mutation.CourseID)
		mutation.PeriodID = strings.TrimSpace(mutation.PeriodID)
		if mutation.CourseID == "" {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "course_id_required", "A course ID is required.", nil)
			return
		}

		var err error
		if r.Method == http.MethodPost {
			if mutation.PeriodID == "" {
				app.writeAPIError(r, w, http.StatusUnprocessableEntity, "period_id_required", "Choose a CCA timetable slot.", nil)
				return
			}
			err = app.queries.NewSelection(r.Context(), db.NewSelectionParams{
				PStudentID:     student.ID,
				PCourseID:      mutation.CourseID,
				PPeriodID:      mutation.PeriodID,
				PSelectionType: db.SelectionTypeNormal,
			})
		} else {
			err = app.queries.DeleteChoiceByStudentAndCourse(r.Context(), db.DeleteChoiceByStudentAndCourseParams{
				PStudentID: student.ID,
				PCourseID:  mutation.CourseID,
			})
		}
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		view, err := app.loadStudentBootstrap(r.Context(), student)
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.broadcastCourseCounts(r, []string{mutation.CourseID})
		app.writeJSON(r, w, http.StatusOK, view)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type adminBootstrapView struct {
	Admin      adminSessionView            `json:"admin"`
	Categories []string                    `json:"categories"`
	Periods    []string                    `json:"periods"`
	Grades     []AbsGradesRow              `json:"grades"`
	Courses    []CourseView                `json:"courses"`
	Students   []db.GetStudentsForAdminRow `json:"students"`
	Selections []db.GetSelectionsRow       `json:"selections"`
}

func (app *App) handleAPIAdminBootstrap(w http.ResponseWriter, r *http.Request, admin *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	categories, err := app.queries.GetCategories(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	grades, err := app.AbsGrades(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	courses, err := app.listCourseViews(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	students, err := app.queries.GetStudentsForAdmin(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	selections, err := app.queries.GetSelections(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	if categories == nil {
		categories = []string{}
	}
	if periods == nil {
		periods = []string{}
	}
	if grades == nil {
		grades = []AbsGradesRow{}
	}
	if courses == nil {
		courses = []CourseView{}
	}
	if students == nil {
		students = []db.GetStudentsForAdminRow{}
	}
	if selections == nil {
		selections = []db.GetSelectionsRow{}
	}
	app.writeJSON(r, w, http.StatusOK, adminBootstrapView{
		Admin:      adminSessionView{ID: admin.ID, Username: admin.Username},
		Categories: categories,
		Periods:    periods,
		Grades:     grades,
		Courses:    courses,
		Students:   students,
		Selections: selections,
	})
}

type coursePayload struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	PeriodIDs         []string          `json:"period_ids"`
	MaxStudents       int64             `json:"max_students"`
	Membership        db.MembershipType `json:"membership"`
	Teacher           string            `json:"teacher"`
	Location          string            `json:"location"`
	CategoryID        string            `json:"category_id"`
	AllowedLegalSexes []db.LegalSex     `json:"allowed_legal_sexes"`
	AllowedGradeIDs   []string          `json:"allowed_grades"`
}

func (payload coursePayload) input(id string) (CourseInput, error) {
	if id == "" {
		id = strings.TrimSpace(payload.ID)
	}
	if id == "" || strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Teacher) == "" || strings.TrimSpace(payload.Location) == "" || strings.TrimSpace(payload.CategoryID) == "" {
		return CourseInput{}, errors.New("id, name, teacher, location, and category_id are required")
	}
	if payload.MaxStudents < 0 {
		return CourseInput{}, errors.New("max_students cannot be negative")
	}
	switch payload.Membership {
	case db.MembershipTypeFree, db.MembershipTypeInviteOnly:
	default:
		return CourseInput{}, errors.New("unknown membership type")
	}
	for _, legalSex := range payload.AllowedLegalSexes {
		switch legalSex {
		case db.LegalSexF, db.LegalSexM, db.LegalSexX:
		default:
			return CourseInput{}, errors.New("unknown legal sex")
		}
	}
	return CourseInput{
		ID:                id,
		Name:              payload.Name,
		Description:       payload.Description,
		PeriodIDs:         payload.PeriodIDs,
		MaxStudents:       payload.MaxStudents,
		Membership:        payload.Membership,
		Teacher:           payload.Teacher,
		Location:          payload.Location,
		CategoryID:        payload.CategoryID,
		AllowedLegalSexes: payload.AllowedLegalSexes,
		AllowedGradeIDs:   payload.AllowedGradeIDs,
	}, nil
}

func (app *App) handleAPIAdminCourses(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	switch r.Method {
	case http.MethodGet:
		courses, err := app.listCourseViews(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.writeJSON(r, w, http.StatusOK, courses)
	case http.MethodPost:
		var payload coursePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		input, err := payload.input("")
		if err != nil {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
			return
		}
		if err := app.createCourse(r.Context(), input); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_courses"))
		app.writeJSON(r, w, http.StatusCreated, map[string]string{"id": input.ID})
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (app *App) handleAPIAdminCourse(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		app.writeAPIError(r, w, http.StatusBadRequest, "course_id_required", "A course ID is required.", nil)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload coursePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		if payload.ID != "" && payload.ID != id {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "course_id_mismatch", "The path and payload course IDs must match.", nil)
			return
		}
		input, err := payload.input(id)
		if err != nil {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
			return
		}
		if err := app.updateCourse(r.Context(), input); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_courses"))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := app.queries.DeleteCourse(r.Context(), id); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_courses"))
		w.WriteHeader(http.StatusNoContent)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type namedResourcePayload struct {
	ID string `json:"id"`
}

func (app *App) handleAPIAdminPeriods(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	periods, err := app.queries.GetPeriods(r.Context())
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	if periods == nil {
		periods = []string{}
	}
	app.writeJSON(r, w, http.StatusOK, periods)
}

func (app *App) handleAPIAdminCategories(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	switch r.Method {
	case http.MethodGet:
		categories, err := app.queries.GetCategories(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if categories == nil {
			categories = []string{}
		}
		app.writeJSON(r, w, http.StatusOK, categories)
	case http.MethodPost:
		var payload namedResourcePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload.ID = strings.TrimSpace(payload.ID)
		if payload.ID == "" {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "category_id_required", "A category ID is required.", nil)
			return
		}
		if err := app.queries.NewCategory(r.Context(), payload.ID); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_categories"))
		app.writeJSON(r, w, http.StatusCreated, payload)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (app *App) handleAPIAdminCategory(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodDelete {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := app.queries.DeleteCategory(r.Context(), id); err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.wsHub.Broadcast(WSMessage("invalidate_categories"))
	w.WriteHeader(http.StatusNoContent)
}
