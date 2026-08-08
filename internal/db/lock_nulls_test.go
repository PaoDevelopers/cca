package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/PaoDevelopers/cca/internal/db"
)

// NULL is not "nothing wanted".
//
// A Go nil slice arrives as a NULL array. Inside the lockers,
// `id = ANY (NULL)` matches nothing and `count(DISTINCT u) FROM
// unnest(NULL)` is also zero, so the counts agree and the existence
// check passes — having locked nothing at all. Every guarantee built
// on those locks then quietly evaporates, on the path where students
// accept nothing.

// A swap that names no course to replace is a plain enroll, and must
// hold the course row exactly as self_enroll does. Without it, two
// students racing for the last seat both read the old count and both
// insert.
func TestSwapWithoutReplacementsStillHoldsTheSeat(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const rivals = 16

	// One seat, many students.
	seedRace(t, pool, rivals, 1)

	errs := race(t, rivals, func(i int) error {
		return q.SelfSwap(context.Background(), db.SelfSwapParams{
			PStudentID:    fmt.Sprintf("s%d", i),
			POldCourseIds: nil, // the wire's absent "replacing"
			PCourseID:     "RACE",
		})
	})

	demandNoDeadlock(t, errs, "swap without replacements")

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`); n != 1 {
		t.Fatalf("the course sold %d of its 1 seat", n)
	}
}

// And the existence contract the lockers advertise must survive a NULL
// too: naming nobody is not the same as naming somebody who is there.
func TestBatchWritesRejectNullArrays(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 10)
	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'RACE', TRUE, TRUE)`)

	ctx := context.Background()

	// The refusal has to be the lockers' own, raised as 22023 by the
	// NULL guard. Asserting only that something went wrong is weaker
	// than it looks: a NULL array also produces a 23502 from a NOT
	// NULL column, and a 22P02 from a cast, in both cases after the
	// function has already skipped the locking this test is about. The
	// error is the evidence, so it is the error that is checked.
	//
	// A removal naming nobody removed nobody; reporting success is a
	// lie an administrator would act on.
	demandNullRejected(t, "remove_enrollments",
		q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
			PCourseID: "RACE", PStudentIds: nil,
		}))

	demandNullRejected(t, "set_enrollment_policy",
		q.SetEnrollmentPolicy(ctx, db.SetEnrollmentPolicyParams{
			PCourseID: "RACE", PStudentIds: nil,
			PStudentDroppable: false, PCountsTowardBudget: false, PAccept: nil,
		}))

	demandNullRejected(t, "place_enrollments",
		q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
			PCourseID: "RACE", PStudentIds: nil,
			PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
		}))

	// And the same for the batches that name their own rows.
	demandNullRejected(t, "upsert_students",
		q.UpsertStudents(ctx, db.UpsertStudentsParams{
			PIds: nil, PNames: nil, PGradeIds: nil,
			PLegalSexes: nil, PAccept: nil,
		}))

	demandNullRejected(t, "set_grade_order", q.SetGradeOrder(ctx, nil))
	demandNullRejected(t, "set_period_order", q.SetPeriodOrder(ctx, nil))

	// Nothing was written by any of them.
	if n := count(t, pool,
		`SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`); n != 1 {
		t.Errorf("a NULL-list write changed the enrollments: %d rows", n)
	}
}

// demandNullRejected insists that a NULL array was refused as 22023 —
// the lockers' own guard — rather than by whatever the function
// stumbled into afterwards.
func demandNullRejected(t *testing.T, what string, err error) {
	t.Helper()

	if err == nil {
		t.Errorf("%s accepted a NULL list as success", what)

		return
	}

	if code := pgCode(err); code != "22023" {
		t.Errorf("%s refused a NULL list with %s, want 22023: %v", what, code, err)
	}
}
