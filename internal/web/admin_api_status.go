package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Where every student stands: budget spent against their cap, distinct
// categories against the minimum, and each requirement group with
// whether it is met.
//
// This is a read model, not a computation. The admin app used to
// derive requirement satisfaction in TypeScript from the enrollments
// and the grade definitions, which was a second implementation of a
// rule the database already states — and the two drift silently. The
// student app asks the server for the same facts through
// /student/api/user_info; this is the administrator's whole-school
// view of them.

// APIStudentStatus is one student's advisory standing.
type APIStudentStatus struct {
	StudentID string `json:"student_id"`
	GradeID   string `json:"grade_id"`

	BudgetedPeriodsUsed int64  `json:"budgeted_periods_used"`
	MaxBudgetedPeriods  *int64 `json:"max_budgeted_periods"`

	DistinctCategoriesUsed int64 `json:"distinct_categories_used"`
	MinDistinctCategories  int64 `json:"min_distinct_categories"`

	Requirements []studentRequirement `json:"requirements"`

	// Every requirement met and the category spread reached. Derived
	// here only because it is an "and" over the rows above, not a
	// rule of its own.
	RequirementsMet bool `json:"requirements_met"`
}

func (app *Server) apiStudentsStatus(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	out, err := app.studentStatus(ctx)
	if err != nil {
		app.apiDBError(r, w, err)

		return
	}

	app.writeJSON(r, w, out, slog.Int("student_count", len(out)))
}

// studentStatus reads every student's standing under one snapshot.
//
// A function of its own so that the transaction is closed by the time
// the response is written. Held across writeJSON, a pool connection
// and an MVCC snapshot belonged to whichever client was slowest to
// read its bytes — which at 1200 students is under the socket buffer
// and so never bites, but the margin is the kernel's, not a decision
// anybody made. Grades already reads this way.
func (app *Server) studentStatus(ctx context.Context) ([]APIStudentStatus, error) {
	// One snapshot across the two reads: a student's status and their
	// requirements must describe the same moment, or a row can show a
	// budget from before an enrollment and requirements from after.
	//exhaustruct:ignore
	tx, err := app.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("begin student status snapshot: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	qtx := app.queries.WithTx(tx)

	rows, err := qtx.GetStudentStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch student status: %w", err)
	}

	// One read for every student's requirements rather than one per
	// student.
	reqs, err := qtx.GetStudentRequirements(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch student requirements: %w", err)
	}

	byStudent := make(map[string][]studentRequirement, len(rows))
	for _, req := range reqs {
		byStudent[req.StudentID] = append(byStudent[req.StudentID], studentRequirement{
			ID:               req.RequirementCategoryID,
			MinPeriodCount:   req.MinPeriodCount,
			SatisfiedPeriods: req.SatisfiedPeriods,
			Met:              req.Met,
		})
	}

	out := make([]APIStudentStatus, len(rows))

	for i, row := range rows {
		list := byStudent[row.StudentID]
		if list == nil {
			list = []studentRequirement{}
		}

		met := row.DistinctCategoriesUsed >= row.MinDistinctCategories
		for _, req := range list {
			met = met && req.Met
		}

		out[i] = APIStudentStatus{
			StudentID:              row.StudentID,
			GradeID:                row.GradeID,
			BudgetedPeriodsUsed:    row.BudgetedPeriodsUsed,
			MaxBudgetedPeriods:     int64Ptr(row.MaxBudgetedPeriods),
			DistinctCategoriesUsed: row.DistinctCategoriesUsed,
			MinDistinctCategories:  row.MinDistinctCategories,
			Requirements:           list,
			RequirementsMet:        met,
		}
	}

	return out, nil
}
