package main

import (
	"context"
	"encoding/json"
	"fmt"

	"git.sr.ht/~runxiyu/cca/db"
)

type CourseBlockReason struct {
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	PeriodIDs         []string `json:"period_ids,omitempty"`
	ConflictingCourse string   `json:"conflicting_course_id,omitempty"`
}

type CourseView struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	PeriodIDs          []string            `json:"period_ids"`
	MaxStudents        int64               `json:"max_students"`
	CurrentStudents    int64               `json:"current_students"`
	StateRevision      int64               `json:"state_revision"`
	Membership         db.MembershipType   `json:"membership"`
	Teacher            string              `json:"teacher"`
	Location           string              `json:"location"`
	CategoryID         string              `json:"category_id"`
	AllowedLegalSexes  []db.LegalSex       `json:"allowed_legal_sexes"`
	AllowedGradeIDs    []string            `json:"allowed_grades"`
	Selected           bool                `json:"selected"`
	SelectedPeriodID   string              `json:"selected_period_id,omitempty"`
	AvailablePeriodIDs []string            `json:"available_period_ids"`
	SelectionType      *db.SelectionType   `json:"selection_type,omitempty"`
	Available          bool                `json:"available"`
	BlockReasons       []CourseBlockReason `json:"block_reasons"`
	Removable          bool                `json:"removable"`
	RemovalBlockReason string              `json:"removal_block_reason,omitempty"`
}

// listCourseViews is the unrestricted administrator read model. Student
// availability must use listStudentCourseViews, whose business rules are
// evaluated by PostgreSQL in a single statement snapshot.
func (app *App) listCourseViews(ctx context.Context) ([]CourseView, error) {
	courses, err := app.queries.GetCourses(ctx)
	if err != nil {
		return nil, err
	}
	legalSexRows, err := app.queries.GetCourseAllowedLegalSexes(ctx)
	if err != nil {
		return nil, err
	}
	gradeRows, err := app.queries.GetCourseAllowedGrades(ctx)
	if err != nil {
		return nil, err
	}

	legalSexes := make(map[string][]db.LegalSex)
	for _, row := range legalSexRows {
		legalSexes[row.CourseID] = append(legalSexes[row.CourseID], row.LegalSex)
	}
	grades := make(map[string][]string)
	for _, row := range gradeRows {
		grades[row.CourseID] = append(grades[row.CourseID], row.Grade)
	}

	result := make([]CourseView, 0, len(courses))
	for _, course := range courses {
		periodIDs := course.PeriodIds
		if periodIDs == nil {
			periodIDs = []string{}
		}
		allowedLegalSexes := legalSexes[course.ID]
		if allowedLegalSexes == nil {
			allowedLegalSexes = []db.LegalSex{}
		}
		allowedGrades := grades[course.ID]
		if allowedGrades == nil {
			allowedGrades = []string{}
		}
		result = append(result, CourseView{
			ID:                 course.ID,
			Name:               course.Name,
			Description:        course.Description,
			PeriodIDs:          periodIDs,
			MaxStudents:        course.MaxStudents,
			CurrentStudents:    course.CurrentStudents,
			StateRevision:      course.StateRevision,
			Membership:         course.Membership,
			Teacher:            course.Teacher,
			Location:           course.Location,
			CategoryID:         course.CategoryID,
			AllowedLegalSexes:  allowedLegalSexes,
			AllowedGradeIDs:    allowedGrades,
			AvailablePeriodIDs: periodIDs,
			Available:          len(periodIDs) > 0,
			BlockReasons:       []CourseBlockReason{},
		})
	}
	return result, nil
}

func (app *App) listStudentCourseViews(ctx context.Context, student *UserInfoStudent) ([]CourseView, error) {
	return listStudentCourseViewsWithQueries(ctx, app.queries, student)
}

func listStudentCourseViewsWithQueries(ctx context.Context, queries *db.Queries, student *UserInfoStudent) ([]CourseView, error) {
	rows, err := queries.GetStudentCourseCatalog(ctx, student.ID)
	if err != nil {
		return nil, err
	}

	result := make([]CourseView, 0, len(rows))
	for _, row := range rows {
		blockReasons := []CourseBlockReason{}
		if err := json.Unmarshal(row.BlockReasons, &blockReasons); err != nil {
			return nil, fmt.Errorf("decode SQL course block reasons for %s: %w", row.ID, err)
		}

		var selectionType *db.SelectionType
		if row.SelectionType.Valid {
			selectionTypeValue := row.SelectionType.SelectionType
			selectionType = &selectionTypeValue
		}
		selectedPeriodID := ""
		if row.SelectedPeriodID.Valid {
			selectedPeriodID = row.SelectedPeriodID.String
		}
		allowedLegalSexes := make([]db.LegalSex, len(row.AllowedLegalSexes))
		for i, legalSex := range row.AllowedLegalSexes {
			allowedLegalSexes[i] = db.LegalSex(legalSex)
		}

		result = append(result, CourseView{
			ID:                 row.ID,
			Name:               row.Name,
			Description:        row.Description,
			PeriodIDs:          row.PeriodIds,
			MaxStudents:        row.MaxStudents,
			CurrentStudents:    row.CurrentStudents,
			StateRevision:      row.StateRevision,
			Membership:         row.Membership,
			Teacher:            row.Teacher,
			Location:           row.Location,
			CategoryID:         row.CategoryID,
			AllowedLegalSexes:  allowedLegalSexes,
			AllowedGradeIDs:    row.AllowedGrades,
			Selected:           row.Selected,
			SelectedPeriodID:   selectedPeriodID,
			AvailablePeriodIDs: row.AvailablePeriodIds,
			SelectionType:      selectionType,
			Available:          row.Available,
			BlockReasons:       blockReasons,
			Removable:          row.Removable,
			RemovalBlockReason: row.RemovalBlockReason,
		})
	}
	return result, nil
}
