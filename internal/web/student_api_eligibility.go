package web

import (
	"log/slog"
	"net/http"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Why the student cannot take each course they do not already hold,
// for every course, in one call.
//
// The alternative was to restate the rules in TypeScript so the
// browser could grey out courses from the catalogue and its own
// enrollments. That is a second implementation of clash, capacity and
// budget, and the two drift silently — the drift is noticed when it
// hides a rejection, not when it is introduced. Asking the database,
// which is where the rules are defined, keeps one definition.
//
// The read is per student, so it is invalidated constantly and shared
// with nobody; that costs nothing, because nobody else was going to
// cache it.

// eligibility is the response: course id to the violations that course
// would produce for this student. Courses the student already holds
// are absent, as are courses they may take freely.
type eligibility map[string][]apiViolation

func (app *Server) handleStuAPIEligibility(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStuAPIEligibility", slog.String("student_id", sui.ID))

	if r.Method != http.MethodGet {
		app.apiMethodNotAllowed(r, w)

		return
	}

	// A student's own enrollment always charges their budget and is
	// always theirs to drop, so the prospective bit is TRUE.
	ctx, cancel := readCtx(r.Context())
	defer cancel()

	rows, err := app.queries.StudentCourseViolations(ctx,
		db.StudentCourseViolationsParams{
			PStudentID:          sui.ID,
			PCountsTowardBudget: true,
		})
	if err != nil {
		app.apiDBError(r, w, err, slog.String("student_id", sui.ID))

		return
	}

	out := make(eligibility)

	for _, row := range rows {
		out[row.CourseID.String] = append(out[row.CourseID.String], apiViolation{
			StudentID:     nil,
			Rule:          row.Rule.String,
			Code:          row.Code.String,
			OtherCourseID: textPtr(row.OtherCourseID),
			PeriodID:      textPtr(row.PeriodID),
			Detail:        row.Detail.String,
		})
	}

	app.writeJSON(r, w, out, slog.String("student_id", sui.ID))
}
