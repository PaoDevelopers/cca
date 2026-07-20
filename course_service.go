package main

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/jackc/pgx/v5"
)

var errCourseNeedsPeriod = errors.New("course must have at least one timetable period")

type CourseInput struct {
	ID                string
	Name              string
	Description       string
	PeriodIDs         []string
	MaxStudents       int64
	Membership        db.MembershipType
	Teacher           string
	Location          string
	CategoryID        string
	AllowedLegalSexes []db.LegalSex
	AllowedGradeIDs   []string
}

func normalizeStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeOrderedStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !slices.Contains(right, value) {
			return false
		}
	}
	return true
}

func normalizeLegalSexSet(values []db.LegalSex) []db.LegalSex {
	seen := make(map[db.LegalSex]struct{}, len(values))
	result := make([]db.LegalSex, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func normalizeCourseInput(input CourseInput) (CourseInput, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Teacher = strings.TrimSpace(input.Teacher)
	input.Location = strings.TrimSpace(input.Location)
	input.CategoryID = strings.TrimSpace(input.CategoryID)
	// Period display order comes from periods.ordinal in SQL. Preserve the
	// submitted order here; period sets are compared as sets below.
	input.PeriodIDs = normalizeOrderedStringSet(input.PeriodIDs)
	input.AllowedGradeIDs = normalizeStringSet(input.AllowedGradeIDs)
	input.AllowedLegalSexes = normalizeLegalSexSet(input.AllowedLegalSexes)
	if len(input.PeriodIDs) == 0 {
		return CourseInput{}, errCourseNeedsPeriod
	}
	return input, nil
}

func (app *App) createCourse(ctx context.Context, input CourseInput) error {
	input, err := normalizeCourseInput(input)
	if err != nil {
		return err
	}

	tx, err := app.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := app.queries.WithTx(tx)
	if err := qtx.NewCourse(ctx, db.NewCourseParams{
		ID:          input.ID,
		Name:        input.Name,
		Description: input.Description,
		MaxStudents: input.MaxStudents,
		Membership:  input.Membership,
		Teacher:     input.Teacher,
		Location:    input.Location,
		CategoryID:  input.CategoryID,
	}); err != nil {
		return err
	}

	if err := writeCourseRelations(ctx, qtx, input, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (app *App) updateCourse(ctx context.Context, input CourseInput) error {
	input, err := normalizeCourseInput(input)
	if err != nil {
		return err
	}

	tx, err := app.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := app.queries.WithTx(tx)
	rowsAffected, err := qtx.UpdateCourse(ctx, db.UpdateCourseParams{
		ID:          input.ID,
		Name:        input.Name,
		Description: input.Description,
		MaxStudents: input.MaxStudents,
		Membership:  input.Membership,
		Teacher:     input.Teacher,
		Location:    input.Location,
		CategoryID:  input.CategoryID,
	})
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return pgx.ErrNoRows
	}

	if err := writeCourseRelations(ctx, qtx, input, true); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func writeCourseRelations(ctx context.Context, qtx *db.Queries, input CourseInput, replace bool) error {
	if replace {
		existing, err := qtx.GetCoursePeriodsByCourse(ctx, input.ID)
		if err != nil {
			return err
		}
		if !equalStringSets(existing, input.PeriodIDs) {
			for _, periodID := range input.PeriodIDs {
				if err := qtx.AddCoursePeriod(ctx, db.AddCoursePeriodParams{
					CourseID: input.ID,
					PeriodID: periodID,
				}); err != nil {
					return err
				}
			}
			if err := qtx.DeleteCoursePeriodsExcept(ctx, db.DeleteCoursePeriodsExceptParams{
				CourseID:  input.ID,
				PeriodIds: input.PeriodIDs,
			}); err != nil {
				return err
			}
		}

		if err := qtx.DeleteCourseAllowedLegalSexes(ctx, input.ID); err != nil {
			return err
		}
		if err := qtx.DeleteCourseAllowedGrades(ctx, input.ID); err != nil {
			return err
		}
	} else {
		for _, periodID := range input.PeriodIDs {
			if err := qtx.AddCoursePeriod(ctx, db.AddCoursePeriodParams{
				CourseID: input.ID,
				PeriodID: periodID,
			}); err != nil {
				return err
			}
		}
	}

	for _, legalSex := range input.AllowedLegalSexes {
		if err := qtx.AddCourseAllowedLegalSex(ctx, db.AddCourseAllowedLegalSexParams{
			CourseID: input.ID,
			LegalSex: legalSex,
		}); err != nil {
			return err
		}
	}
	for _, gradeID := range input.AllowedGradeIDs {
		if err := qtx.AddCourseAllowedGrade(ctx, db.AddCourseAllowedGradeParams{
			CourseID: input.ID,
			Grade:    gradeID,
		}); err != nil {
			return err
		}
	}
	return nil
}
