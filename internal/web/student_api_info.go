package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// What the student is and where they stand: their name and grade, the
// budget they have spent against their cap, the categories they span
// against the minimum, and each requirement group with whether it is
// met.
//
// All of it is advisory except the cap, and even the cap is enforced
// by the write functions rather than by anything here. This endpoint
// exists so the student can see what is expected of them before they
// run into it.
type studentInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	GradeID string `json:"grade_id"`

	BudgetedPeriodsUsed int64  `json:"budgeted_periods_used"`
	MaxBudgetedPeriods  *int64 `json:"max_budgeted_periods"`

	DistinctCategoriesUsed int64 `json:"distinct_categories_used"`
	MinDistinctCategories  int64 `json:"min_distinct_categories"`

	Requirements []studentRequirement `json:"requirements"`
}

type studentRequirement struct {
	ID               int64 `json:"id"`
	MinPeriodCount   int64 `json:"min_period_count"`
	SatisfiedPeriods int64 `json:"satisfied_periods"`
	Met              bool  `json:"met"`
}

func (app *Server) handleStuAPIInfo(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIInfo", slog.String("student_id", sui.ID))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	ctx, cancel := readCtx(r.Context())
	defer cancel()

	status, err := app.queries.GetStudentStatusByID(ctx, sui.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The session is well signed but names nobody: the
			// student was deleted from the roster since they signed
			// in. That is a gone session, not a server fault.
			app.apiError(r, w, http.StatusUnauthorized, codeUnauthenticated,
				"your account is no longer in the student roster", err,
				slog.String("student_id", sui.ID))

			return
		}

		app.apiDBError(r, w, err, slog.String("student_id", sui.ID))

		return
	}

	reqs, err := app.queries.GetStudentRequirementsByID(ctx, sui.ID)
	if err != nil {
		app.apiDBError(r, w, err, slog.String("student_id", sui.ID))

		return
	}

	// The frontend expects [] rather than null.
	list := make([]studentRequirement, len(reqs))
	for i, req := range reqs {
		list[i] = studentRequirement{
			ID:               req.RequirementCategoryID,
			MinPeriodCount:   req.MinPeriodCount,
			SatisfiedPeriods: req.SatisfiedPeriods,
			Met:              req.Met,
		}
	}

	app.writeJSON(r, w, studentInfo{
		ID:                     status.StudentID,
		Name:                   status.StudentName,
		GradeID:                status.GradeID,
		BudgetedPeriodsUsed:    status.BudgetedPeriodsUsed,
		MaxBudgetedPeriods:     int64Ptr(status.MaxBudgetedPeriods),
		DistinctCategoriesUsed: status.DistinctCategoriesUsed,
		MinDistinctCategories:  status.MinDistinctCategories,
		Requirements:           list,
	}, slog.String("student_id", sui.ID))
}
