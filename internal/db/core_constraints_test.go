package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The core data model's unconditional constraints:
// domains, checks, keys.
// The subject is the raw table surface,
// so everything here speaks SQL directly, not the query layer.

// expectExec runs sql expecting failure with exactly this SQLSTATE.
func expectExec(t *testing.T, pool *pgxpool.Pool, state, sql string, args ...any) {
	t.Helper()

	_, err := pool.Exec(context.Background(), sql, args...)
	if err == nil {
		t.Fatalf("expected SQLSTATE %s but statement succeeded: %s", state, sql)
	}

	expectState(t, err, state)
}

func TestEntityIDGrammar(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	// Uppercase alphanumerics, '-', '_', 1..16 chars.
	for _, id := range []string{
		"sport",             // lowercase
		"A:B",               // colon (violation-code delimiter)
		"A B",               // space
		"",                  // empty
		"ABCDEFGHIJKLMNOPQ", // 17 chars
	} {
		expectExec(t, pool, "23514",
			`INSERT INTO categories (id, name) VALUES ($1, $2)`, id, "N"+id)
	}

	exec(t, pool, `INSERT INTO categories (id, name) VALUES
		('ABCDEFGHIJKLMNOP', 'Sixteen'),  -- 16 chars: boundary accept
		('SPORT', 'Sports 体育')`) // CJK name accept
}

func TestTrimmedTextRejectsPaddingAndControls(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	for _, name := range []string{
		"", " Padded", "Padded ", "\tTab", "Tab\t", "Two\nLines",
	} {
		expectExec(t, pool, "23514",
			`INSERT INTO categories (id, name) VALUES ('T1', $1)`, name)
	}

	exec(t, pool,
		`INSERT INTO categories (id, name) VALUES ('T7', 'Interior spaces are fine')`)

	// Names are unique per table.
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports 体育')`)
	expectExec(t, pool, "23505",
		`INSERT INTO categories (id, name) VALUES ('SPORT2', 'Sports 体育')`)
}

func TestGradeWindowConstraints(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	// The window must close after it opens when both bounds are
	// present; either bound may be absent; the cap may be NULL.
	exec(t, pool, `INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
		('Y9', 'Year 9', NULL, NULL, NULL, 0, 1),
		('Y10', 'Year 10', now(), now() + interval '1 day', 8, 2, 2),
		('Y11', 'Year 11', NULL, now(), 0, 0, 3),
		('Y12', 'Year 12', now(), NULL, NULL, 0, 4)`)

	expectExec(t, pool, "23514",
		`INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('Y13', 'Year 13', now(), now() - interval '1 hour', NULL, 0, 5)`) // inverted
	expectExec(t, pool, "23514",
		`INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('Y13', 'Year 13', now(), now(), NULL, 0, 5)`) // empty window
	expectExec(t, pool, "23514",
		`INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('Y13', 'Year 13', -1, 0, 5)`) // negative cap
}

func TestLocalpartGrammar(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	exec(t, pool,
		`INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('Y9', 'Year 9', NULL, 0, 1)`)

	for _, id := range []string{
		"S22537",    // uppercase
		".runxi",    // leading dot
		"runxi.",    // trailing dot
		"runxi..yu", // consecutive dots
		"s22537@x",  // not a bare localpart
	} {
		expectExec(t, pool, "23514",
			`INSERT INTO students (id, name, grade_id, legal_sex) VALUES ($1, 'X', 'Y9', 'F')`, id)
	}

	expectExec(t, pool, "23514",
		`INSERT INTO students (id, name, grade_id, legal_sex)
		VALUES (repeat('a', 65), 'X', 'Y9', 'F')`) // 65 chars

	exec(t, pool, `INSERT INTO students (id, name, grade_id, legal_sex) VALUES
		(repeat('a', 64), 'Boundary', 'Y9', 'F'),
		('s22537', 'Zhang San', 'Y9', 'F'),
		('runxi.yu', 'Runxi Yu', 'Y9', 'X')`) // staff account for testing

	// Students reference real grades.
	expectExec(t, pool, "23503",
		`INSERT INTO students (id, name, grade_id, legal_sex) VALUES ('s1', 'G', 'Y99', 'F')`)
}

func TestCourseFieldConstraints(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports')`)

	// Optional display fields take '' but never padding.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('WEBDEV', 'Web Development', '', 20, FALSE,
			'', '', '', 'Season', '', 'SPORT')`)
	expectExec(t, pool, "23514", `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('X1', 'X', '', 20, FALSE, ' Ms Li', '', '', 'Season', '', 'SPORT')`)
	expectExec(t, pool, "23514", `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('X2', 'X', '', -1, FALSE, '', '', '', 'Season', '', 'SPORT')`) // negative capacity
	// term is optional like location and cost: a department that does
	// not divide its season into terms leaves the column empty, and
	// nothing in the software reads the value. It still refuses
	// padding.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('X3', 'X', '', 20, FALSE, '', '', '', '', '', 'SPORT')`)
	expectExec(t, pool, "23514", `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('X4', 'X', '', 20, FALSE, '', '', '', 'Season ', '', 'SPORT')`)

	// Same name as WEBDEV: course names are deliberately not
	// unique; 0 capacity is legal.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('CHESS', 'Web Development', '', 0, TRUE,
			'Ms Li', 'ms.li@example.org', 'Room 2', 'Season', '600 rmb', 'SPORT')`)

	// Category deletion is restricted while courses reference it.
	expectExec(t, pool, "23503", `DELETE FROM categories WHERE id = 'SPORT'`)
}

func TestEnrollmentKeysAndCascades(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', NULL, 0, 1);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s22537', 'Zhang San', 'Y9', 'F'),
			('runxi.yu', 'Runxi Yu', 'Y9', 'X');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id)
			VALUES ('WEBDEV', 'A', '', 20, FALSE, '', '', '', 'Season', '', 'SPORT'),
				('CHESS', 'B', '', 20, FALSE, '', '', '', 'Season', '', 'SPORT')`)

	// All four policy combinations are legal.
	exec(t, pool, `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
		('s22537', 'WEBDEV', TRUE, TRUE),
		('s22537', 'CHESS', TRUE, FALSE),
		('runxi.yu', 'WEBDEV', FALSE, FALSE),
		('runxi.yu', 'CHESS', FALSE, TRUE)`)

	// One enrollment per student per course.
	expectExec(t, pool, "23505", `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s22537', 'WEBDEV', FALSE, FALSE)`)
	// Enrollments reference real students and courses.
	expectExec(t, pool, "23503", `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('nobody', 'WEBDEV', TRUE, TRUE)`)
	expectExec(t, pool, "23503", `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s22537', 'NOPE', TRUE, TRUE)`)
	// A student with enrollments cannot be deleted from under them.
	expectExec(t, pool, "23503", `DELETE FROM students WHERE id = 's22537'`)

	// Renaming an id cascades to referencing rows.
	exec(t, pool, `UPDATE courses SET id = 'WEBDEV1' WHERE id = 'WEBDEV'`)
	exec(t, pool, `UPDATE grades SET id = 'Y09' WHERE id = 'Y9'`)

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'WEBDEV1'`); n != 2 {
		t.Fatalf("course rename cascaded to %d enrollments, want 2", n)
	}

	if n := count(t, pool, `SELECT count(*) FROM students WHERE grade_id = 'Y09'`); n != 2 {
		t.Fatalf("grade rename cascaded to %d students, want 2", n)
	}
}

func TestSortOrderCarriesNoInvariant(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	// Display order is declared as a whole, so sort_order is only
	// ever a number: duplicates and gaps are legal states, and the
	// order is still definite because ties break by id.
	exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES
		('TUE1', 'Tuesday 16:00-17:00', 5),
		('MON1', 'Monday 16:00-17:00', 5),
		('WED1', 'Wednesday 16:00-17:00', 9)`)

	var order pgtype.Text
	if err := pool.QueryRow(context.Background(),
		`SELECT string_agg(id, ',' ORDER BY sort_order, id) FROM periods`).
		Scan(&order); err != nil {
		t.Fatalf("order: %v", err)
	}

	if order.String != "MON1,TUE1,WED1" {
		t.Fatalf("ties and gaps must still yield one definite order: %s",
			order.String)
	}

	// Zero and below are not positions.
	expectExec(t, pool, "23514", `INSERT INTO periods (id, name, sort_order)
		VALUES ('THU1', 'Thursday 16:00-17:00', 0)`)
}

func TestSchemaVersionSingleton(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	expectExec(t, pool, "23505",
		`INSERT INTO schema_version (singleton, version) VALUES (TRUE, 2)`)
	expectExec(t, pool, "23514",
		`INSERT INTO schema_version (singleton, version) VALUES (FALSE, 2)`)
}
