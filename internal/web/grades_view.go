package web

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The grade document both areas read. Administrators edit every field
// of it; students read the window and the two advisory numbers to know
// what they may do and what is still expected of them.
//
// Nullability is meaningful in two places and is carried as JSON null
// rather than a sentinel: a null bound means "no such bound" (an
// unopened or never-closing window), and a null cap means "no cap".
// Zero means zero in both.

// APIRequirement is one requirement group of a grade.
type APIRequirement struct {
	ID             int64    `json:"id"`
	MinPeriodCount int64    `json:"min_period_count"`
	CategoryIDs    []string `json:"category_ids"`
}

// APIGrade is a grade with its requirements.
type APIGrade struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	OpensAt  *time.Time `json:"opens_at"`
	ClosesAt *time.Time `json:"closes_at"`

	// Derived from the two bounds by v_grades, never computed here:
	// display and enforcement read the same definition, so a student
	// cannot be shown an open window the write functions will refuse.
	IsOpen bool `json:"is_open"`

	MaxBudgetedPeriods    *int64 `json:"max_budgeted_periods"`
	MinDistinctCategories int64  `json:"min_distinct_categories"`
	SortOrder             int64  `json:"sort_order"`

	Requirements []APIRequirement `json:"requirements"`
}

// Grades runs its two reads in one read-only repeatable-read
// transaction: a grade added or a requirement edited between them
// would otherwise produce a document that never existed — a grade
// listed without the requirements it was created with, say.
func (app *Server) Grades(ctx context.Context) ([]APIGrade, error) {
	//exhaustruct:ignore
	tx, err := app.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin grades snapshot: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	return grades(ctx, app.queries.WithTx(tx))
}

// Takes a queries handle so a caller already in a transaction can
// reuse it.
func grades(ctx context.Context, queries *db.Queries) ([]APIGrade, error) {
	rows, err := queries.GetGrades(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch grades: %w", err)
	}

	// One read for every grade's requirements rather than one per
	// grade.
	reqs, err := queries.GetGradeRequirements(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch grade requirements: %w", err)
	}

	byGrade := make(map[string][]APIRequirement, len(rows))
	for _, req := range reqs {
		byGrade[req.GradeID] = append(byGrade[req.GradeID], APIRequirement{
			ID:             req.ID,
			MinPeriodCount: req.MinPeriodCount,
			CategoryIDs:    req.CategoryIds,
		})
	}

	out := make([]APIGrade, len(rows))
	for i, row := range rows {
		// Both frontends expect [] rather than null.
		list := byGrade[row.ID]
		if list == nil {
			list = []APIRequirement{}
		}

		out[i] = APIGrade{
			ID:                    row.ID,
			Name:                  row.Name,
			OpensAt:               timePtr(row.OpensAt),
			ClosesAt:              timePtr(row.ClosesAt),
			IsOpen:                row.IsOpen.Valid && row.IsOpen.Bool,
			MaxBudgetedPeriods:    int64Ptr(row.MaxBudgetedPeriods),
			MinDistinctCategories: row.MinDistinctCategories,
			SortOrder:             row.SortOrder,
			Requirements:          list,
		}
	}

	return out, nil
}
