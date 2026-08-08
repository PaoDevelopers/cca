package db_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// enrollment_violations: every rule on both sides of its boundary,
// code serialization, and the disregard (swap) semantics,
// read through the production query.

// violationCodes runs the violations query and returns codes,
// sorted, optionally filtered to one rule.
func violationCodes(t *testing.T, q *db.Queries, rule string,
	arg db.EnrollmentViolationsParams,
) []string {
	t.Helper()

	rows, err := q.EnrollmentViolations(context.Background(), arg)
	if err != nil {
		t.Fatalf("EnrollmentViolations: %v", err)
	}

	var codes []string

	for _, r := range rows {
		if !r.Rule.Valid || !r.Code.Valid {
			t.Fatalf("rule and code must never be NULL: %+v", r)
		}

		if rule == "" || r.Rule.String == rule {
			codes = append(codes, r.Code.String)
		}
	}

	slices.Sort(codes)

	return codes
}

func violations(student, course string, charging bool, disregard ...string) db.EnrollmentViolationsParams {
	return db.EnrollmentViolationsParams{
		PStudentID:          student,
		PCourseID:           course,
		PCountsTowardBudget: charging,
		PDisregardCourseIds: disregard,
	}
}

// seedViolations:
// TWICE: SPORT, MON1+TUE1, roomy, unrestricted.
// ARTMON: ART, MON1 (clashes with TWICE), roomy, unrestricted.
// TINY: capacity 1, unrestricted, WED1.
// GIRLS: allows F only; Y9GRADE: allows Y9 only; both WED1, roomy.
// s1 (F, Y9, cap 4) takes TWICE charging 2;
// s2 (M, Y9) takes the only TINY seat, not charging;
// s3 (M, Y10, no cap) takes nothing.
func seedViolations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', 4, 0, 1),
			('Y10', 'Year 10', NULL, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2), ('WED1', 'Wednesday', 3);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'Y9', 'F'),
			('s2', 'Student Two', 'Y9', 'M'),
			('s3', 'Student Three', 'Y10', 'M');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('TWICE', 'Twice a week', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('ARTMON', 'Monday Art', '', 10, FALSE, '', '', '', 'Season', '', 'ART'),
			('TINY', 'One seat', '', 1, FALSE, '', '', '', 'Season', '', 'ART'),
			('GIRLS', 'Girls only', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('Y9GRADE', 'Year 9 only', '', 10, FALSE, '', '', '', 'Season', '', 'ART');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1'),
			('ARTMON', 'MON1'), ('TINY', 'WED1'),
			('GIRLS', 'WED1'), ('Y9GRADE', 'WED1');
		INSERT INTO course_allowed_legal_sexes (course_id, legal_sex) VALUES ('GIRLS', 'F');
		INSERT INTO course_allowed_grades (course_id, grade_id) VALUES ('Y9GRADE', 'Y9');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
			('s1', 'TWICE', TRUE, TRUE),
			('s2', 'TINY', TRUE, FALSE)`)
}

func TestFitRules(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// A clean placement violates nothing.
	if c := violationCodes(t, q, "", violations("s2", "ARTMON", true)); len(c) != 0 {
		t.Fatalf("clean placement produced %v", c)
	}

	// legal_sex: boundary on both sides, only with a non-empty list.
	// (s2 also clashes in WED1 via TINY; asserts filter per rule.)
	if c := violationCodes(t, q, "legal_sex", violations("s2", "GIRLS", false)); !slices.Equal(c, []string{"legal_sex:s2:GIRLS"}) {
		t.Fatalf("M against F-only: %v", c)
	}

	if c := violationCodes(t, q, "", violations("s1", "GIRLS", false)); len(c) != 0 {
		t.Fatalf("F against F-only must pass: %v", c)
	}

	// grade: likewise.
	if c := violationCodes(t, q, "", violations("s3", "Y9GRADE", false)); !slices.Equal(c, []string{"grade:s3:Y9GRADE"}) {
		t.Fatalf("Y10 against Y9-only: %v", c)
	}

	if c := violationCodes(t, q, "grade", violations("s2", "Y9GRADE", false)); len(c) != 0 {
		t.Fatalf("Y9 against Y9-only must pass: %v", c)
	}

	// capacity: full is a violation, the last seat is not
	// (s1 took the roomy TWICE, TINY's one seat is s2's).
	if c := violationCodes(t, q, "", violations("s1", "TINY", false)); !slices.Equal(c, []string{"capacity:s1:TINY"}) {
		t.Fatalf("full course: %v", c)
	}

	// clash: one row per clashing (course, period),
	// self-identifying code plus structured subjects.
	rows, err := q.EnrollmentViolations(context.Background(),
		violations("s1", "ARTMON", false))
	if err != nil {
		t.Fatalf("EnrollmentViolations: %v", err)
	}

	if len(rows) != 1 || rows[0].Code.String != "clash:s1:ARTMON:TWICE:MON1" ||
		rows[0].OtherCourseID.String != "TWICE" || rows[0].PeriodID.String != "MON1" {
		t.Fatalf("clash row: %+v", rows)
	}

	// disregard: a swap dropping TWICE frees MON1.
	if c := violationCodes(t, q, "", violations("s1", "ARTMON", false, "TWICE")); len(c) != 0 {
		t.Fatalf("disregarding the dropped course must clear its clash: %v", c)
	}
}

func TestBudgetRule(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// TWICE2 gives a 2-period course s1 is not in yet;
	// TUE1 also clashes with TWICE, so asserts filter to budget.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('TWICE2', 'Second twice a week', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE2', 'WED1'), ('TWICE2', 'TUE1')`)

	// 2 used + 2 new = 4 of 4: exactly at the cap passes.
	if c := violationCodes(t, q, "budget", violations("s1", "TWICE2", true)); len(c) != 0 {
		t.Fatalf("reaching the cap exactly must pass: %v", c)
	}

	// One more charged period pushes over.
	exec(t, pool, `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'ARTMON', TRUE, TRUE)`)

	if c := violationCodes(t, q, "budget", violations("s1", "TWICE2", true)); !slices.Equal(c, []string{"budget:s1"}) {
		t.Fatalf("exceeding the cap: %v", c)
	}

	// The same placement not charging is budget-exempt.
	if c := violationCodes(t, q, "budget", violations("s1", "TWICE2", false)); len(c) != 0 {
		t.Fatalf("non-charging placement must never violate budget: %v", c)
	}

	// Non-charging enrollments never count as used:
	// pile s2 with 3 non-charging periods (TINY + TWICE),
	// so miscounting them (3 + 2 > 4) would cross the cap.
	exec(t, pool, `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s2', 'TWICE', TRUE, FALSE)`)

	if c := violationCodes(t, q, "budget", violations("s2", "TWICE2", true)); len(c) != 0 {
		t.Fatalf("non-charging enrollments must not count toward usage: %v", c)
	}

	// A NULL cap is no cap.
	if c := violationCodes(t, q, "budget", violations("s3", "TWICE2", true)); len(c) != 0 {
		t.Fatalf("a NULL cap must never violate: %v", c)
	}

	// Disregard also frees budget: dropping TWICE (2 charged
	// periods) in the same motion brings s1 back under the cap.
	if c := violationCodes(t, q, "budget", violations("s1", "TWICE2", true, "TWICE")); len(c) != 0 {
		t.Fatalf("disregarded courses must not charge the budget: %v", c)
	}
}

func TestAllViolationsReportTogether(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// s2 (M, in a WED1 course) against a full, F-only,
	// WED1-clashing GIRLS: every violated rule reports at once.
	exec(t, pool, `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'GIRLS', TRUE, FALSE);
		UPDATE courses SET max_students = 1 WHERE id = 'GIRLS'`)

	want := []string{"capacity:s2:GIRLS", "clash:s2:GIRLS:TINY:WED1", "legal_sex:s2:GIRLS"}
	if c := violationCodes(t, q, "", violations("s2", "GIRLS", false)); !slices.Equal(c, want) {
		t.Fatalf("got %v, want %v", c, want)
	}
}
