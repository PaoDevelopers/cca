package db_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The enrollment write functions: gates on the student path,
// the accept protocol on the admin path, swap atomicity,
// batch walk semantics.

// seedWrites: OPEN grade's window is open, SHUT grade's is closed.
// SWIM (2 seats) and ARTMON share MON1; ARTTUE is clash-free;
// CLUB is invite-only with no periods.
func seedWrites(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('OPEN', 'Open grade', now() - interval '1 hour', now() + interval '1 hour', 4, 0, 1),
			('SHUT', 'Shut grade', NULL, NULL, 4, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2), ('WED1', 'Wednesday', 3);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Open One', 'OPEN', 'F'),
			('s2', 'Open Two', 'OPEN', 'M'),
			('s3', 'Shut One', 'SHUT', 'F');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('SWIM', 'Swimming', '', 2, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('CLUB', 'Invite club', '', 10, TRUE, '', '', '', 'Season', '', 'ART'),
			('ARTMON', 'Monday Art', '', 10, FALSE, '', '', '', 'Season', '', 'ART'),
			('ARTTUE', 'Tuesday Art', '', 10, FALSE, '', '', '', 'Season', '', 'ART');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('SWIM', 'MON1'), ('ARTMON', 'MON1'), ('ARTTUE', 'TUE1')`)
}

func selfEnroll(q *db.Queries, student, course string) error {
	if err := q.SelfEnroll(context.Background(), db.SelfEnrollParams{
		PStudentID: student, PCourseID: course,
	}); err != nil {
		return fmt.Errorf("self_enroll: %w", err)
	}

	return nil
}

func selfDrop(q *db.Queries, student, course string) error {
	if err := q.SelfDrop(context.Background(), db.SelfDropParams{
		PStudentID: student, PCourseID: course,
	}); err != nil {
		return fmt.Errorf("self_drop: %w", err)
	}

	return nil
}

func enrolledCourses(t *testing.T, q *db.Queries, student string) []string {
	t.Helper()

	rows, err := q.GetEnrollmentsByStudent(context.Background(), student)
	if err != nil {
		t.Fatalf("GetEnrollmentsByStudent: %v", err)
	}

	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.CourseID)
	}

	return ids
}

func TestSelfEnrollAndDrop(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	// Happy path creates a droppable, charging row.
	if err := selfEnroll(q, "s1", "SWIM"); err != nil {
		t.Fatalf("self_enroll: %v", err)
	}

	rows, err := q.GetEnrollmentsByStudent(context.Background(), "s1")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v, err %v", rows, err)
	}

	if !rows[0].StudentDroppable || !rows[0].CountsTowardBudget {
		t.Fatalf("self-enrollment must be droppable and charging: %+v", rows[0])
	}

	// Gates, each with its own state.
	expectState(t, selfEnroll(q, "s3", "SWIM"), "YKG01")    // window shut
	expectState(t, selfEnroll(q, "s2", "CLUB"), "YKG02")    // invite-only
	expectState(t, selfEnroll(q, "s2", "NOPE"), "P0002")    // no such course
	expectState(t, selfEnroll(q, "ghost", "SWIM"), "P0002") // no such student
	expectState(t, selfEnroll(q, "s1", "SWIM"), "23505")    // already enrolled

	// Students accept nothing: a clash refuses as YKV01,
	// leaving no row.
	expectCodes(t, selfEnroll(q, "s1", "ARTMON"), "clash:s1:ARTMON:SWIM:MON1")

	if got := enrolledCourses(t, q, "s1"); !slices.Equal(got, []string{"SWIM"}) {
		t.Fatalf("a refused self-enrollment must leave no row: %v", got)
	}

	// self_drop: gates and outcomes.
	expectState(t, selfDrop(q, "s1", "ARTMON"), "P0002") // not enrolled

	if err := selfDrop(q, "s1", "SWIM"); err != nil {
		t.Fatalf("self_drop: %v", err)
	}

	if got := enrolledCourses(t, q, "s1"); len(got) != 0 {
		t.Fatalf("self_drop must delete the row: %v", got)
	}

	// A non-droppable row refuses with YKG03,
	// and the window gate binds drops too.
	exec(t, pool, `INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'SWIM', FALSE, FALSE), ('s3', 'ARTMON', TRUE, TRUE)`)
	expectState(t, selfDrop(q, "s1", "SWIM"), "YKG03")
	expectState(t, selfDrop(q, "s3", "ARTMON"), "YKG01")
}

func TestSelfSwap(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	ctx := context.Background()
	swap := func(student string, old []string, course string) error {
		return q.SelfSwap(ctx, db.SelfSwapParams{
			PStudentID: student, POldCourseIds: old, PCourseID: course,
		})
	}

	if err := selfEnroll(q, "s1", "SWIM"); err != nil {
		t.Fatalf("self_enroll: %v", err)
	}

	// Swapping SWIM for ARTMON succeeds only because SWIM is
	// disregarded: they share MON1.
	if err := swap("s1", []string{"SWIM"}, "ARTMON"); err != nil {
		t.Fatalf("swap: %v", err)
	}

	if got := enrolledCourses(t, q, "s1"); !slices.Equal(got, []string{"ARTMON"}) {
		t.Fatalf("the swap must drop the old row and add the new: %v", got)
	}

	// A refused swap changes nothing: swap back but against an
	// invite-only target.
	expectState(t, swap("s1", []string{"ARTMON"}, "CLUB"), "YKG02")

	if got := enrolledCourses(t, q, "s1"); !slices.Equal(got, []string{"ARTMON"}) {
		t.Fatalf("a refused swap must leave enrollments untouched: %v", got)
	}

	// Old rows must exist and be droppable.
	expectState(t, swap("s1", []string{"SWIM"}, "ARTTUE"), "P0002")
	exec(t, pool, `UPDATE enrollments SET student_droppable = FALSE
		WHERE student_id = 's1' AND course_id = 'ARTMON'`)
	expectState(t, swap("s1", []string{"ARTMON"}, "SWIM"), "YKG03")
}

func TestPlaceAndRemoveEnrollments(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	ctx := context.Background()
	place := func(course string, students []string, droppable, charging bool, accept ...string) error {
		return q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: students,
			PStudentDroppable: droppable, PCountsTowardBudget: charging,
			PAccept: accept,
		})
	}

	// The batch is atomic: SWIM holds 2, so the third element
	// violates capacity (earlier elements win the seats,
	// the code names the loser) and everything rolls back.
	expectCodes(t, place("SWIM", []string{"s1", "s2", "s3"}, true, false),
		"capacity:s3:SWIM")

	if n := count(t, pool, `SELECT count(*) FROM enrollments`); n != 0 {
		t.Fatalf("a refused batch must leave no rows; left %d", n)
	}

	// With the violation accepted, the whole batch lands,
	// including the over-capacity row, with the bits as given.
	if err := place("SWIM", []string{"s1", "s2", "s3"}, true, false,
		"capacity:s3:SWIM"); err != nil {
		t.Fatalf("accepted batch: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND NOT counts_toward_budget AND student_droppable`); n != 3 {
		t.Fatalf("landed rows with given bits = %d, want 3", n)
	}

	// A stale accept is ignored, not an error.
	if err := place("ARTTUE", []string{"s3"}, false, false, "capacity:s3:SWIM"); err != nil {
		t.Fatalf("a stale accept must not block a clean placement: %v", err)
	}

	// Novel violations block a resubmission naming another.
	expectCodes(t, place("ARTMON", []string{"s1"}, false, false, "capacity:s1:ARTMON"),
		"clash:s1:ARTMON:SWIM:MON1")

	// No gates on the admin path: s3's window is shut and CLUB is
	// invite-only, yet placement proceeds.
	if err := place("CLUB", []string{"s3"}, false, false); err != nil {
		t.Fatalf("admin placement must ignore window and membership gates: %v", err)
	}

	if got := enrolledCourses(t, q, "s3"); !slices.Equal(got, []string{"ARTTUE", "CLUB", "SWIM"}) {
		t.Fatalf("s3's enrollments: %v", got)
	}

	// remove_enrollments removes anything, including non-droppable
	// rows; missing rows are P0002.
	exec(t, pool, `UPDATE enrollments SET student_droppable = FALSE
		WHERE course_id = 'SWIM' AND student_id = 's1'`)

	if err := q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1", "s2"},
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'SWIM'`); n != 1 {
		t.Fatalf("removal must delete exactly the named enrollments; %d left", n)
	}

	expectState(t, q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
	}), "P0002")
}
