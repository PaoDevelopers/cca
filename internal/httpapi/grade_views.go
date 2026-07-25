package httpapi

import (
	"context"
	"fmt"

	"git.sr.ht/~runxiyu/cca/internal/store/sqlc"
)

// AbsGradesRow combines a grade with its requirement groups.
type AbsGradesRow struct {
	Grade         string                              `json:"grade"`
	Enabled       bool                                `json:"enabled"`
	MaxOwnChoices int64                               `json:"max_own_choices"`
	ReqGroups     []db.GetRequirementGroupsByGradeRow `json:"req_groups"`
}

// AbsGrades returns grades and their requirement groups.
func (app *App) AbsGrades(ctx context.Context) ([]AbsGradesRow, error) {
	return absGradesWithQueries(ctx, app.queries)
}

func absGradesWithQueries(ctx context.Context, queries *db.Queries) ([]AbsGradesRow, error) {
	grades2 := []AbsGradesRow{}

	grades, err := queries.GetGrades(ctx)
	if err != nil {
		return grades2, fmt.Errorf("fetch grades: %w", err)
	}

	for _, grade := range grades {
		reqGroups, err := queries.GetRequirementGroupsByGrade(ctx, grade.Grade)
		if err != nil {
			return grades2, fmt.Errorf("fetch grade requirements: %w", err)
		}
		if reqGroups == nil {
			reqGroups = []db.GetRequirementGroupsByGradeRow{}
		}
		grades2 = append(grades2, AbsGradesRow{
			Grade:         grade.Grade,
			Enabled:       grade.Enabled,
			MaxOwnChoices: grade.MaxOwnChoices,
			ReqGroups:     reqGroups,
		})
	}

	return grades2, nil
}
