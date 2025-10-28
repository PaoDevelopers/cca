package main

import (
	"context"
	"fmt"

	"git.sr.ht/~runxiyu/cca/db"
)

type AbsGradesRow struct {
	Grade     string                              `json:"grade"`
	Enabled   bool                                `json:"enabled"`
	ReqGroups []db.GetRequirementGroupsByGradeRow `json:"req_groups"`
}

func (app *App) AbsGrades(ctx context.Context) ([]AbsGradesRow, error) {
	// TODO: Transactions! And maybe get db queries thing from caller? I think maybe all abstract functions should do so
	grades2 := []AbsGradesRow{}

	grades, err := app.queries.GetGrades(ctx)
	if err != nil {
		return grades2, fmt.Errorf("Cannot fetch grades: %w", err)
	}

	for _, grade := range grades {
		reqGroups, err := app.queries.GetRequirementGroupsByGrade(ctx, grade.Grade)
		if err != nil {
			return grades2, fmt.Errorf("Cannot fetch grade requirements: %w", err)
		}
		grades2 = append(grades2, AbsGradesRow{
			grade.Grade,
			grade.Enabled,
			reqGroups,
		})
	}

	return grades2, nil
}
