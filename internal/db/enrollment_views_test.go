package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The enrollment read models: name embedding, budget accounting,
// distinct-category counting, requirement satisfaction.

// seedEnrollmentViews: TWICE meets twice a week (charges 2),
// ONCE once (charges 1), UNSCHED has no periods (charges 0,
// satisfies nothing). s1 holds an own pick charging 2, an invited
// non-charging ART placement, and an unscheduled charging pick;
// s2 has no enrollments at all.
func seedEnrollmentViews(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES
			('SPORT', 'Sports'), ('ART', 'Art'), ('CULTURE', 'Culture');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', 4, 2, 1);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday 16:00-17:00', 1),
			('TUE1', 'Tuesday 16:00-17:00', 2),
			('WED1', 'Wednesday 16:00-17:00', 3);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'Y9', 'F'),
			('s2', 'Student Two', 'Y9', 'M');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('TWICE', 'Twice-a-week Sport', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('ONCE', 'Weekly Art', '', 10, FALSE, '', '', '', 'Season', '', 'ART'),
			('UNSCHED', 'Unscheduled Culture', '', 10, FALSE, '', '', '', 'Season', '', 'CULTURE');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1'), ('ONCE', 'WED1');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
			('s1', 'TWICE', TRUE, TRUE),
			('s1', 'ONCE', TRUE, FALSE),
			('s1', 'UNSCHED', TRUE, TRUE)`)
}

func TestEnrollmentViewEmbedsNames(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedEnrollmentViews(t, pool)

	rows, err := q.GetEnrollmentsByStudent(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetEnrollmentsByStudent: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("%d rows, want 3", len(rows))
	}

	// ORDER BY course_id: ONCE, TWICE, UNSCHED.
	r := rows[1]
	if r.StudentName != "Student One" || r.GradeID != "Y9" ||
		r.CourseName != "Twice-a-week Sport" {
		t.Fatalf("embedded names wrong: %+v", r)
	}

	if !r.StudentDroppable || !r.CountsTowardBudget {
		t.Fatalf("policy bits must ride along: %+v", r)
	}
}

func TestStudentStatusDerivations(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedEnrollmentViews(t, pool)

	ctx := context.Background()

	// Budget: TWICE charges 2, ONCE is not charging,
	// UNSCHED charges 0.
	s1, err := q.GetStudentStatusByID(ctx, "s1")
	if err != nil {
		t.Fatalf("status s1: %v", err)
	}

	if s1.BudgetedPeriodsUsed != 2 {
		t.Fatalf("budget used = %d, must count periods of charging enrollments only",
			s1.BudgetedPeriodsUsed)
	}

	if !s1.MaxBudgetedPeriods.Valid || s1.MaxBudgetedPeriods.Int64 != 4 {
		t.Fatalf("the grade cap must ride along: %+v", s1.MaxBudgetedPeriods)
	}

	// Distinct categories: SPORT and ART are scheduled, CULTURE is not.
	if s1.DistinctCategoriesUsed != 2 {
		t.Fatalf("distinct categories = %d; unscheduled courses must not count",
			s1.DistinctCategoriesUsed)
	}

	// A student with no enrollments still has a status row of zeros.
	s2, err := q.GetStudentStatusByID(ctx, "s2")
	if err != nil {
		t.Fatalf("status s2: %v", err)
	}

	if s2.BudgetedPeriodsUsed != 0 || s2.DistinctCategoriesUsed != 0 {
		t.Fatalf("enrollment-free students must read as zeros: %+v", s2)
	}
}

func TestRequirementSatisfaction(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedEnrollmentViews(t, pool)

	ctx := context.Background()

	// SPORTREQ: 2 periods in SPORT (s1 meets it exactly).
	// BROAD: 4 periods across all three categories (s1 has 3, unmet).
	// EMPTYSET: 0 periods across no categories (trivially met;
	// unrepresentable through the write layer, seeded raw).
	if err := setRequirements(t, q, "Y9",
		requirement{2, []string{"SPORT"}},
		requirement{4, []string{"SPORT", "ART", "CULTURE"}}); err != nil {
		t.Fatalf("requirements: %v", err)
	}

	sportreq := requirementID(t, pool, "Y9", 2)
	broad := requirementID(t, pool, "Y9", 4)

	var emptyset int64
	if err := pool.QueryRow(ctx, `INSERT INTO grade_requirement_groups
		(grade_id, min_period_count) VALUES ('Y9', 0)
		RETURNING id`).Scan(&emptyset); err != nil {
		t.Fatalf("emptyset: %v", err)
	}

	rows, err := q.GetStudentRequirements(ctx)
	if err != nil {
		t.Fatalf("GetStudentRequirements: %v", err)
	}

	if len(rows) != 6 {
		t.Fatalf("%d rows; every student must cross every requirement of their grade", len(rows))
	}

	get := func(student string, req int64) db.VStudentRequirement {
		for _, r := range rows {
			if r.StudentID == student && r.RequirementCategoryID == req {
				return r
			}
		}

		t.Fatalf("row %s/%d missing", student, req)

		return db.VStudentRequirement{}
	}

	if r := get("s1", sportreq); r.SatisfiedPeriods != 2 || !r.Met {
		t.Fatalf("meeting a requirement exactly must count as met: %+v", r)
	}

	// The invited ART placement must satisfy requirements;
	// unscheduled CULTURE must not.
	if r := get("s1", broad); r.SatisfiedPeriods != 3 || r.Met {
		t.Fatalf("occupancy-is-occupancy violated: %+v", r)
	}

	if r := get("s1", emptyset); r.SatisfiedPeriods != 0 || !r.Met {
		t.Fatalf("an empty member set with a zero minimum is trivially met: %+v", r)
	}

	if r := get("s2", sportreq); r.SatisfiedPeriods != 0 || r.Met {
		t.Fatalf("no enrollments satisfies nothing: %+v", r)
	}
}
