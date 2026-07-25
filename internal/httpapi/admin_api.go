package httpapi

import (
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.sr.ht/~runxiyu/cca/internal/store/sqlc"
	"github.com/jackc/pgx/v5"
)

type gradePayload struct {
	Grade         string `json:"grade"`
	Enabled       bool   `json:"enabled"`
	MaxOwnChoices int64  `json:"max_own_choices"`
}

type gradeUpdatePayload struct {
	Grade         string `json:"grade"`
	Enabled       *bool  `json:"enabled"`
	MaxOwnChoices int64  `json:"max_own_choices"`
}

func (app *App) handleAPIAdminGrades(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	switch r.Method {
	case http.MethodGet:
		grades, err := app.AbsGrades(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.writeJSON(r, w, http.StatusOK, grades)
	case http.MethodPost:
		var payload gradePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload.Grade = strings.TrimSpace(payload.Grade)
		if payload.Grade == "" || payload.MaxOwnChoices < 0 {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "A grade and a non-negative selection limit are required.", nil)
			return
		}
		tx, err := app.pool.Begin(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		qtx := app.queries.WithTx(tx)
		if err := qtx.NewGrade(r.Context(), db.NewGradeParams{Grade: payload.Grade, MaxOwnChoices: payload.MaxOwnChoices}); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if payload.Enabled {
			rowsAffected, err := qtx.UpdateGradeSettings(r.Context(), db.UpdateGradeSettingsParams{Grade: payload.Grade, Enabled: true, MaxOwnChoices: payload.MaxOwnChoices})
			if err != nil {
				app.writeClassifiedAPIError(r, w, err)
				return
			}
			if rowsAffected == 0 {
				app.writeClassifiedAPIError(r, w, pgx.ErrNoRows)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		app.writeJSON(r, w, http.StatusCreated, payload)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (app *App) handleAPIAdminGrade(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	grade := strings.TrimSpace(r.PathValue("grade"))
	if grade == "" {
		app.writeAPIError(r, w, http.StatusBadRequest, "grade_required", "A grade is required.", nil)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload gradeUpdatePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		if payload.MaxOwnChoices < 0 {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "The selection limit cannot be negative.", nil)
			return
		}
		updated, err := app.updateGradeSettings(r.Context(), grade, payload.Enabled, payload.MaxOwnChoices)
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if !updated {
			app.writeClassifiedAPIError(r, w, pgx.ErrNoRows)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := app.queries.DeleteGrade(r.Context(), grade); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		w.WriteHeader(http.StatusNoContent)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type requirementGroupPayload struct {
	MinCount    int64    `json:"min_count"`
	CategoryIDs []string `json:"category_ids"`
}

func (app *App) handleAPIAdminRequirementGroups(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	grade := strings.TrimSpace(r.PathValue("grade"))
	var payload requirementGroupPayload
	if err := decodeAPIJSON(w, r, &payload); err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
		return
	}
	payload.CategoryIDs = normalizeStringSet(payload.CategoryIDs)
	if grade == "" || payload.MinCount < 0 || len(payload.CategoryIDs) == 0 {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "A grade, non-negative minimum, and at least one category are required.", nil)
		return
	}
	if err := app.queries.NewRequirementGroup(r.Context(), db.NewRequirementGroupParams{
		Grade: grade, MinCount: payload.MinCount, CategoryIds: payload.CategoryIDs,
	}); err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.writeJSON(r, w, http.StatusCreated, payload)
}

func (app *App) handleAPIAdminRequirementGroup(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodDelete {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_requirement_group_id", "Invalid requirement-group ID.", err)
		return
	}
	if err := app.queries.DeleteRequirementGroup(r.Context(), id); err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	w.WriteHeader(http.StatusNoContent)
}

type studentPayload struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	Grade    string      `json:"grade"`
	LegalSex db.LegalSex `json:"legal_sex"`
}

func validateStudentPayload(payload studentPayload, id int64) (studentPayload, error) {
	if id != 0 {
		payload.ID = id
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Grade = strings.TrimSpace(payload.Grade)
	if payload.ID <= 0 || payload.Name == "" || payload.Grade == "" {
		return studentPayload{}, errInvalidStudent
	}
	switch payload.LegalSex {
	case db.LegalSexF, db.LegalSexM, db.LegalSexX:
	default:
		return studentPayload{}, errInvalidStudent
	}
	return payload, nil
}

var errInvalidStudent = &validationError{message: "A positive ID, name, grade, and valid legal sex are required."}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }

func (app *App) handleAPIAdminStudents(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	switch r.Method {
	case http.MethodGet:
		students, err := app.queries.GetStudentsForAdmin(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.writeJSON(r, w, http.StatusOK, students)
	case http.MethodPost:
		var payload studentPayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload, err := validateStudentPayload(payload, 0)
		if err != nil {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
			return
		}
		if err := app.queries.NewStudent(r.Context(), db.NewStudentParams(payload)); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.writeJSON(r, w, http.StatusCreated, payload)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (app *App) handleAPIAdminStudent(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_student_id", "Invalid student ID.", err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload studentPayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload, err = validateStudentPayload(payload, id)
		if err != nil {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
			return
		}
		rowsAffected, err := app.queries.UpdateStudent(r.Context(), db.UpdateStudentParams(payload))
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if rowsAffected == 0 {
			app.writeClassifiedAPIError(r, w, pgx.ErrNoRows)
			return
		}
		app.wsHub.BroadcastToStudents([]int64{id}, WSMessage("invalidate_student"))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := app.queries.DeleteStudent(r.Context(), id); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type selectionBatchPayload struct {
	StudentIDs    []int64          `json:"student_ids"`
	CourseIDs     []string         `json:"course_ids"`
	PeriodIDs     []string         `json:"period_ids"`
	SelectionType db.SelectionType `json:"selection_type"`
}

func (app *App) handleAPIAdminSelections(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	switch r.Method {
	case http.MethodGet:
		selections, err := app.queries.GetSelections(r.Context())
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.writeJSON(r, w, http.StatusOK, selections)
	case http.MethodPost:
		var payload selectionBatchPayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		slices.Sort(payload.StudentIDs)
		payload.StudentIDs = slices.Compact(payload.StudentIDs)
		switch payload.SelectionType {
		case db.SelectionTypeNormal, db.SelectionTypeInvite, db.SelectionTypeForce:
		default:
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Unknown selection type.", nil)
			return
		}
		if len(payload.StudentIDs) == 0 || len(payload.CourseIDs) == 0 {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Select at least one student and one course.", nil)
			return
		}
		if len(payload.CourseIDs) != len(payload.PeriodIDs) {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Each selected course needs one timetable slot.", nil)
			return
		}
		seenCourses := make(map[string]struct{}, len(payload.CourseIDs))
		for index := range payload.CourseIDs {
			payload.CourseIDs[index] = strings.TrimSpace(payload.CourseIDs[index])
			payload.PeriodIDs[index] = strings.TrimSpace(payload.PeriodIDs[index])
			if payload.CourseIDs[index] == "" || payload.PeriodIDs[index] == "" {
				app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Each selected course needs one timetable slot.", nil)
				return
			}
			if _, exists := seenCourses[payload.CourseIDs[index]]; exists {
				app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Choose only one timetable slot per course.", nil)
				return
			}
			seenCourses[payload.CourseIDs[index]] = struct{}{}
		}
		batch := cartesianSelectionBatch(payload.StudentIDs, payload.CourseIDs, payload.PeriodIDs, payload.SelectionType)
		created, err := app.queries.NewSelectionsBatch(r.Context(), batch)
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.BroadcastToStudents(payload.StudentIDs, WSMessage("invalidate_selections"))
		app.publishCourseStates(r, payload.CourseIDs)
		app.writeJSON(r, w, http.StatusCreated, map[string]int64{"created": created})
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type selectionUpdatePayload struct {
	CourseID      string           `json:"course_id"`
	PeriodID      string           `json:"period_id"`
	SelectionType db.SelectionType `json:"selection_type"`
}

func (app *App) handleAPIAdminSelection(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	studentID, err := strconv.ParseInt(r.PathValue("student_id"), 10, 64)
	if err != nil || studentID <= 0 {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_student_id", "Invalid student ID.", err)
		return
	}
	currentCourseID := strings.TrimSpace(r.PathValue("course_id"))
	if currentCourseID == "" {
		app.writeAPIError(r, w, http.StatusBadRequest, "course_id_required", "A course ID is required.", nil)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload selectionUpdatePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload.CourseID = strings.TrimSpace(payload.CourseID)
		payload.PeriodID = strings.TrimSpace(payload.PeriodID)
		if payload.CourseID == "" {
			payload.CourseID = currentCourseID
		}
		if payload.PeriodID == "" {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "period_id_required", "Choose a CCA timetable slot.", nil)
			return
		}
		switch payload.SelectionType {
		case db.SelectionTypeNormal, db.SelectionTypeInvite, db.SelectionTypeForce:
		default:
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "Unknown selection type.", nil)
			return
		}
		rowsAffected, err := app.queries.UpdateSelection(r.Context(), db.UpdateSelectionParams{
			StudentID: studentID, CurrentCourseID: currentCourseID, NewCourseID: payload.CourseID, NewPeriodID: payload.PeriodID, SelectionType: payload.SelectionType,
		})
		if err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		if rowsAffected == 0 {
			app.writeClassifiedAPIError(r, w, pgx.ErrNoRows)
			return
		}
		app.wsHub.BroadcastToStudents([]int64{studentID}, WSMessage("invalidate_selections"))
		courseIDs := normalizeStringSet([]string{currentCourseID, payload.CourseID})
		app.publishCourseStates(r, courseIDs)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := app.queries.DeleteSelection(r.Context(), db.DeleteSelectionParams{StudentID: studentID, CourseID: currentCourseID}); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.BroadcastToStudents([]int64{studentID}, WSMessage("invalidate_selections"))
		app.publishCourseStates(r, []string{currentCourseID})
		w.WriteHeader(http.StatusNoContent)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

type notificationPayload struct {
	Message string `json:"message"`
}

func (app *App) handleAPIAdminNotifications(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var payload notificationPayload
	if err := decodeAPIJSON(w, r, &payload); err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
		return
	}
	payload.Message = strings.TrimSpace(payload.Message)
	if payload.Message == "" || len(payload.Message) > 1000 {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", "A message of up to 1000 characters is required.", nil)
		return
	}
	app.wsHub.Broadcast(WSMessage("notify," + payload.Message))
	w.WriteHeader(http.StatusNoContent)
}

func sortSelectionRows(rows []db.GetSelectionsRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].StudentID == rows[j].StudentID {
			return rows[i].CourseID < rows[j].CourseID
		}
		return rows[i].StudentID < rows[j].StudentID
	})
}
