package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PaoDevelopers/cca/internal/db"
)

// set_max_budgeted_periods: apply-then-judge scoped to budget,
// one violation per affected student, other grades untouched.

func TestSetMaxBudgetedPeriods(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	// G1: s1 charges 2 periods, s2 charges 1, s3 charges nothing.
	// G2's t1 charges 2 against a cap of 1 —
	// a standing over-cap that must never leak into G1's judgments.
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('G1', 'Grade One', 4, 0, 1),
			('G2', 'Grade Two', 1, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Charging Two', 'G1', 'F'),
			('s2', 'Charging One', 'G1', 'M'),
			('s3', 'Not Charging', 'G1', 'M'),
			('t1', 'Other Grade', 'G2', 'F');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('TWICE', 'Two periods', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('ONCE', 'One period', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1'), ('ONCE', 'MON1');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
			('s1', 'TWICE', TRUE, TRUE),
			('s2', 'ONCE', TRUE, TRUE),
			('s3', 'ONCE', TRUE, FALSE),
			('t1', 'TWICE', TRUE, TRUE)`)

	ctx := context.Background()
	set := func(grade string, limit pgtype.Int8, accept ...string) error {
		return q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
			GradeID: grade, MaxBudgetedPeriods: limit, Accept: accept,
		})
	}
	capOf := func(n int64) pgtype.Int8 { return pgtype.Int8{Int64: n, Valid: true} }
	noCap := pgtype.Int8{}

	// Shrinking G1 to 1 pushes s1 (2 charged) over;
	// s2 sits at the new cap exactly, s3 charges nothing,
	// and G2's standing over-cap t1 must not appear.
	expectCodes(t, set("G1", capOf(1)), "budget:s1")

	if n := count(t, pool, `SELECT max_budgeted_periods FROM grades WHERE id = 'G1'`); n != 4 {
		t.Fatalf("a refused cap change must not be stored; cap %d", n)
	}

	// Accepted, it applies.
	if err := set("G1", capOf(1), "budget:s1"); err != nil {
		t.Fatalf("accepted shrink: %v", err)
	}

	if n := count(t, pool, `SELECT max_budgeted_periods FROM grades WHERE id = 'G1'`); n != 1 {
		t.Fatalf("an accepted cap change must be stored; cap %d", n)
	}

	// Raising needs no accepts; NULL clears the cap entirely —
	// which only works because the parameter is nullable through
	// the boundary (sqlc.narg).
	if err := set("G1", capOf(4)); err != nil {
		t.Fatalf("raise: %v", err)
	}

	if err := set("G1", noCap); err != nil {
		t.Fatalf("NULL must be storable as no-cap: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'G1' AND max_budgeted_periods IS NULL`); n != 1 {
		t.Fatal("cap must read back as NULL")
	}

	expectState(t, set("NOPE", capOf(1)), "P0002")
}
