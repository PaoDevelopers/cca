package db_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The course read model: live enrollee counts, association
// aggregation, sort_order-driven array ordering.

// seedCourseViews builds the anti-accidental seed:
// sort orders opposing lexical order on both axes,
// one bare course and one carrying every association.
func seedCourseViews(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', NULL, 0, 1),
			('Y10', 'Year 10', NULL, 0, 2);
			-- Y9 deliberately sorts before Y10,
			-- the reverse of their lexical order ('Y10' < 'Y9')
		INSERT INTO periods (id, name, sort_order) VALUES
			('TUE1', 'Tuesday 16:00-17:00', 1),
			('MON1', 'Monday 16:00-17:00', 2);
			-- likewise TUE1 before MON1
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'Y9', 'F'),
			('s2', 'Student Two', 'Y9', 'M');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('BARE', 'Bare', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('FULL', 'Full-featured', 'Longer prose', 2, TRUE,
				'Ms Li', 'ms.li@example.org', 'Gym', 'Season', '600 rmb', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('FULL', 'MON1'), ('FULL', 'TUE1');
		INSERT INTO course_allowed_legal_sexes (course_id, legal_sex) VALUES
			('FULL', 'X'), ('FULL', 'F');
		INSERT INTO course_allowed_grades (course_id, grade_id) VALUES
			('FULL', 'Y9'), ('FULL', 'Y10');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
			('s1', 'FULL', TRUE, TRUE),
			('s2', 'FULL', FALSE, FALSE)`)
}

func courseByID(t *testing.T, q *db.Queries, id string) db.VCourse {
	t.Helper()

	courses, err := q.GetCourses(context.Background())
	if err != nil {
		t.Fatalf("GetCourses: %v", err)
	}

	for _, c := range courses {
		if c.ID == id {
			return c
		}
	}

	t.Fatalf("course %s missing from v_courses", id)

	return db.VCourse{}
}

func TestCourseViewEmptyAndPopulatedShapes(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseViews(t, pool)

	courses, err := q.GetCourses(context.Background())
	if err != nil {
		t.Fatalf("GetCourses: %v", err)
	}

	if len(courses) != 2 {
		t.Fatalf("v_courses carries %d courses, want 2", len(courses))
	}

	// The empty cases: zero count and empty (never nil) arrays.
	bare := courseByID(t, q, "BARE")
	if bare.CurrentStudents != 0 {
		t.Fatalf("BARE count = %d, want 0", bare.CurrentStudents)
	}

	for name, arr := range map[string]int{
		"period_ids":          len(bare.PeriodIds),
		"allowed_legal_sexes": len(bare.AllowedLegalSexes),
		"allowed_grade_ids":   len(bare.AllowedGradeIds),
	} {
		if arr != 0 {
			t.Fatalf("BARE %s not empty", name)
		}
	}

	if bare.PeriodIds == nil || bare.AllowedLegalSexes == nil || bare.AllowedGradeIds == nil {
		t.Fatal("empty associations must read as empty arrays, not NULL")
	}

	// The populated cases.
	full := courseByID(t, q, "FULL")
	if full.CurrentStudents != 2 {
		t.Fatalf("every enrollment counts, whatever its policy bits; got %d", full.CurrentStudents)
	}

	if !slices.Equal(full.PeriodIds, []string{"TUE1", "MON1"}) {
		t.Fatalf("periods = %v, must follow periods.sort_order, not id order", full.PeriodIds)
	}

	if !slices.Equal(full.AllowedLegalSexes, []db.LegalSex{db.LegalSexF, db.LegalSexX}) {
		t.Fatalf("legal sexes = %v, must aggregate in enum order", full.AllowedLegalSexes)
	}

	if !slices.Equal(full.AllowedGradeIds, []string{"Y9", "Y10"}) {
		t.Fatalf("grades = %v, must follow grades.sort_order, not id order", full.AllowedGradeIds)
	}
}

func TestCourseViewTracksWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseViews(t, pool)

	// The count is live: enrollment writes move it immediately.
	exec(t, pool, `DELETE FROM enrollments WHERE student_id = 's2' AND course_id = 'FULL'`)

	if c := courseByID(t, q, "FULL"); c.CurrentStudents != 1 {
		t.Fatalf("count = %d after deletion, want 1", c.CurrentStudents)
	}

	// Array order is presentation order:
	// an administrator reordering periods reorders the arrays.
	exec(t, pool, `UPDATE periods SET sort_order = 3 WHERE id = 'TUE1'`)

	if c := courseByID(t, q, "FULL"); !slices.Equal(c.PeriodIds, []string{"MON1", "TUE1"}) {
		t.Fatalf("periods = %v after reorder, want [MON1 TUE1]", c.PeriodIds)
	}
}
