package db_test

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"testing"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Reproductions for the findings of the schema audit, written before
// any fix, so that each one is demonstrated rather than assumed.

// Reordering takes a row lock on every row it renumbers, in whatever
// order the plan visits them — which is the caller's array order. Two
// callers with differently sorted lists therefore hold each other's
// next row.
func TestReorderingDoesNotDeadlockAgainstGradeWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 4, 100)
	exec(t, pool, `INSERT INTO grades (id, name, min_distinct_categories, sort_order)
		VALUES ('A9', 'A', 0, 2), ('Z9', 'Z', 0, 3)`)

	forward := []string{"A9", "Y9", "Z9"}
	backward := []string{"Z9", "Y9", "A9"}

	errs := race(t, 24, func(i int) error {
		ctx := context.Background()

		switch i % 3 {
		case 0:
			return q.SetGradeOrder(ctx, forward)
		case 1:
			return q.SetGradeOrder(ctx, backward)
		default:
			return q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
				GradeID: "Y9", MaxBudgetedPeriods: pgInt8(int64(i)), Accept: []string{},
			})
		}
	})

	demandNoDeadlock(t, errs, "reordering against grade writes")
}

func TestPeriodReorderingDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 100)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order)
		VALUES ('AAA1', 'A', 2), ('ZZZ1', 'Z', 3)`)

	forward := []string{"AAA1", "MON1", "ZZZ1"}
	backward := []string{"ZZZ1", "MON1", "AAA1"}

	errs := race(t, 24, func(i int) error {
		if i%2 == 0 {
			return q.SetPeriodOrder(context.Background(), forward)
		}

		return q.SetPeriodOrder(context.Background(), backward)
	})

	demandNoDeadlock(t, errs, "opposing period reorders")
}

// Deleting a period takes the period row, then (through the RESTRICT
// probe) the course_periods rows referring to it. update_course goes
// the other way: it holds the course, rewrites course_periods, and
// then takes a key-share lock on the periods it names.
func TestDeletingAPeriodDoesNotDeadlockAgainstRescheduling(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 100)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order)
		VALUES ('SPARE', 'Spare', 9), ('KEEP', 'Keep', 8)`)

	errs := race(t, 24, func(i int) error {
		ctx := context.Background()

		if i%2 == 0 {
			// Churns course_periods, taking key-share on the periods
			// it names. SPARE may be gone by then; that is a plain
			// rejection, not what this test is about.
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1", "SPARE", "KEEP"}, 100))
		}

		// The other direction: the period row, then its referrers.
		if _, err := q.DeletePeriod(ctx, "SPARE"); err != nil {
			return fmt.Errorf("delete_period: %w", err)
		}

		return nil
	})

	demandNoDeadlock(t, errs, "period deletion against rescheduling")
}

// Replacing a grade's requirements is delete-then-insert with no lock
// on the grade, so two concurrent saves produce the union of both
// forms rather than the later one.
func TestConcurrentRequirementSavesDoNotUnion(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 100)
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('ART', 'Art')`)

	one := []byte(`[{"min_period_count": 1, "category_ids": ["SPORT"]}]`)
	two := []byte(`[{"min_period_count": 2, "category_ids": ["ART"]}]`)

	errs := race(t, 20, func(i int) error {
		payload := one
		if i%2 == 0 {
			payload = two
		}

		return q.SetGradeRequirements(context.Background(),
			db.SetGradeRequirementsParams{PGradeID: "Y9", PRequirements: payload})
	})

	demandNoDeadlock(t, errs, "concurrent requirement saves")

	// Whichever form won, the grade holds exactly one requirement:
	// each call states the whole arrangement.
	if n := count(t, pool, `SELECT count(*) FROM grade_requirement_groups WHERE grade_id = 'Y9'`); n != 1 {
		t.Fatalf("the grade holds %d requirements; concurrent whole-set saves unioned", n)
	}
}

// A clash is a fact about a pair of courses in a period. Its code must
// be the same string whichever course the write happened to judge
// from, or an accepted clash comes back unaccepted the next time it is
// surfaced from the other side.
func TestClashCodesAreTheSameFromEitherSide(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 1, 100)
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('OTHER', 'Other', '', 100, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES ('OTHER', 'MON1');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'RACE', TRUE, TRUE)`)

	// Judged from OTHER, with RACE as the incumbent.
	fromOther := violationCodes(t, q, "clash", violations("s1", "OTHER", true))

	// Now the other way round: s1 holds OTHER, and RACE is judged.
	exec(t, pool, `DELETE FROM enrollments WHERE student_id = 's1';
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'OTHER', TRUE, TRUE)`)

	fromRace := violationCodes(t, q, "clash", violations("s1", "RACE", true))

	if !slices.Equal(fromOther, fromRace) {
		t.Fatalf("one clash has two codes:\n  judged from OTHER: %v\n  judged from RACE:  %v",
			fromOther, fromRace)
	}
}

// The consequence of the above, on the path that matters: an
// administrator accepts a clash when placing a student, then edits the
// other course. The clash they already consented to must not come back
// asking again.
func TestAnAcceptedClashStaysAcceptedFromTheOtherSide(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 1, 100)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ('TUE1', 'Tuesday', 2);
		INSERT INTO courses (id, name, description, max_students, invite_only,
			teacher, teacher_email, location, term, cost, category_id)
		VALUES ('OTHER', 'Other', '', 100, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES ('OTHER', 'MON1');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'RACE', TRUE, TRUE)`)

	// Place into the clashing course, accepting whatever it reports.
	err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "OTHER", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: []string{},
	})

	accepted := make([]string, 0, 2)
	for _, v := range expectCodesAny(t, err) {
		accepted = append(accepted, v.Code)
	}

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "OTHER", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: accepted,
	}); err != nil {
		t.Fatalf("the placement was refused even with its own codes accepted: %v", err)
	}

	// Now reschedule the *other* course of the pair. Its enrollees are
	// re-judged, so the same standing clash is surfaced — this time
	// with RACE as the subject rather than OTHER. Naming the codes the
	// administrator already accepted must still be enough.
	updateErr := q.UpdateCourse(context.Background(), db.UpdateCourseParams{
		PCourseID: "RACE", PName: "Contested", PCategoryID: "SPORT",
		PTerm: "Season", PMaxStudents: 100,
		PPeriodIds: []string{"MON1", "TUE1"},
		PAccept:    accepted,
	})
	if updateErr != nil {
		t.Fatalf("an already-accepted clash came back from the other side: %v", updateErr)
	}
}

// expectCodesAny decodes a YKV01 payload without insisting on which
// codes it holds.
func expectCodesAny(t *testing.T, err error) []violation {
	t.Helper()

	pgErr := pgError(t, err, "YKV01")

	var vs []violation
	if e := json.Unmarshal([]byte(pgErr.Detail), &vs); e != nil {
		t.Fatalf("decode DETAIL %q: %v", pgErr.Detail, e)
	}

	return vs
}

// Saving a grade's requirements writes rows referencing categories, so
// it key-shares them — after it has taken the grade. Every course
// write goes the other way, because lock_vocabulary follows the
// doctrine: categories, then periods, then grades. Two administrators,
// one editing requirements and one editing a course, therefore each
// hold what the other needs next.
//
// This is the one that made the doctrine's phrasing matter. Nothing in
// set_grade_requirements' source names a category lock; the insert
// takes it. A rule read off the PERFORMs would have said this function
// was ordered correctly.
func TestRequirementSavesDoNotDeadlockAgainstCourseWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 100)
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('ART', 'Art')`)

	both := []byte(`[{"min_period_count": 1, "category_ids": ["ART", "SPORT"]}]`)

	errs := race(t, 24, func(i int) error {
		ctx := context.Background()

		if i%2 == 0 {
			return q.SetGradeRequirements(ctx,
				db.SetGradeRequirementsParams{PGradeID: "Y9", PRequirements: both})
		}

		// lock_vocabulary: the category, the periods, then the
		// grades the course is restricted to — which is what puts a
		// grade lock on this side of the pairing at all.
		course := raceCourse([]string{"MON1"}, 100)
		course.PGradeIds = []string{"Y9"}

		return q.UpdateCourse(ctx, course)
	})

	demandNoDeadlock(t, errs, "requirement saves against course writes")
}

// A category that does not exist should be reported as a missing row,
// not as whichever foreign key the insert loop happened to reach
// first — and the report must arrive before anything has been deleted.
func TestSavingRequirementsOverAMissingCategoryChangesNothing(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 1, 10)

	ctx := context.Background()

	good := []byte(`[{"min_period_count": 1, "category_ids": ["SPORT"]}]`)
	if err := q.SetGradeRequirements(ctx,
		db.SetGradeRequirementsParams{PGradeID: "Y9", PRequirements: good}); err != nil {
		t.Fatalf("the first save failed: %v", err)
	}

	bad := []byte(`[{"min_period_count": 1, "category_ids": ["NOSUCH"]}]`)

	err := q.SetGradeRequirements(ctx,
		db.SetGradeRequirementsParams{PGradeID: "Y9", PRequirements: bad})
	if err == nil {
		t.Fatal("a requirement over a category that does not exist was accepted")
	}

	if code := pgCode(err); code != "P0002" {
		t.Errorf("SQLSTATE = %s, want P0002 (no_data_found); got %v", code, err)
	}

	// The delete is inside the same transaction as the failure, so the
	// grade still holds what it held before.
	if n := count(t, pool,
		`SELECT count(*) FROM grade_requirement_groups WHERE grade_id = 'Y9'`); n != 1 {
		t.Errorf("the grade holds %d requirements; the refused save destroyed the old form", n)
	}
}
