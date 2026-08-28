package db_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Concurrency properties of the write functions.
// Serial SQL tests cannot catch overselling or deadlocks;
// these run real parallel sessions against committed state.

// seedRace prepares an open grade, n students s1..sn, and one course
// with the given capacity meeting in MON1.
func seedRace(t *testing.T, pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}, students int, capacity int,
) {
	t.Helper()

	ctx := context.Background()

	for _, s := range []string{
		`INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports')`,
		`INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', now() - interval '1 hour', now() + interval '1 hour', NULL, 0, 1)`,
		`INSERT INTO periods (id, name, sort_order) VALUES ('MON1', 'Monday', 1)`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `INSERT INTO courses (id, name, description, max_students, invite_only,
		teacher, teacher_email, location, term, cost, category_id)
		VALUES ('RACE', 'Contested', '', $1, FALSE, '', '', '', 'Season', '', 'SPORT')`,
		capacity); err != nil {
		t.Fatalf("seed course: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO course_periods (course_id, period_id) VALUES ('RACE', 'MON1')`); err != nil {
		t.Fatalf("seed course period: %v", err)
	}

	for i := 1; i <= students; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO students (id, name, grade_id, legal_sex) VALUES ($1, $2, 'Y9', 'F')`,
			fmt.Sprintf("s%d", i), fmt.Sprintf("Student %d", i)); err != nil {
			t.Fatalf("seed student: %v", err)
		}
	}
}

// race runs fn(i) for i in 1..n concurrently and returns the errors.
func race(t *testing.T, n int, fn func(i int) error) []error {
	t.Helper()

	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			errs[i] = fn(i + 1)
		})
	}

	wg.Wait()

	return errs
}

// TestCapacityHoldsUnderConcurrentEnrollment races every student into
// the last seat: exactly one wins, every loser sees the capacity
// violation, and the count never oversells.
func TestCapacityHoldsUnderConcurrentEnrollment(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 16
	seedRace(t, pool, students, 1)

	errs := race(t, students, func(i int) error {
		return q.SelfEnroll(context.Background(), db.SelfEnrollParams{
			PStudentID: fmt.Sprintf("s%d", i),
			PCourseID:  "RACE",
		})
	})

	wins := 0

	for i, err := range errs {
		if err == nil {
			wins++

			continue
		}

		v := expectCodes(t, err, fmt.Sprintf("capacity:s%d:RACE", i+1))
		if v[0].Rule != "capacity" {
			t.Fatalf("loser %d: rule %q", i+1, v[0].Rule)
		}
	}

	if wins != 1 {
		t.Fatalf("%d students won 1 seat", wins)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`); n != 1 {
		t.Fatalf("enrollments = %d, want 1", n)
	}
}

// raceCourse restates the RACE course as seeded, with the timetable
// and capacity the caller wants to move it to. Editing is
// declarative, so a caller that wants to change one thing still
// names the whole course.
func raceCourse(periods []string, seats int64) db.UpdateCourseParams {
	return db.UpdateCourseParams{
		PCourseID:    "RACE",
		PName:        "Contested",
		PCategoryID:  "SPORT",
		PTerm:        "Season",
		PMaxStudents: capacity(seats),
		PPeriodIds:   periods,
		PAccept:      []string{},
	}
}

// TestWriteFunctionsDoNotDeadlock hammers the same students and course
// with every write shape at once (placements, removals, self drops,
// reschedules, cap changes) and demands no 40P01 ever surfaces:
// the documented lock order makes deadlock impossible.
func TestWriteFunctionsDoNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 8
	seedRace(t, pool, students, 1000)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ('TUE1', 'Tuesday', 2)`)

	ids := make([]string, students)
	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
	}

	errs := race(t, 4*students, func(i int) error {
		ctx := context.Background()

		switch i % 4 {
		case 0:
			return q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
				PCourseID:           "RACE",
				PStudentIds:         ids,
				PStudentDroppable:   true,
				PCountsTowardBudget: false,
				PAccept:             []string{},
			})
		case 1:
			return q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
				PCourseID:   "RACE",
				PStudentIds: ids[:students/2],
			})
		case 2:
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1", "TUE1"}, 500))
		default:
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1"}, int64(500+i)))
		}
	})

	for _, err := range errs {
		if err == nil {
			continue
		}

		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
			if pgErr.Code == "40P01" {
				t.Fatalf("deadlock: %s", pgErr.Message)
			}
			// Expected turbulence: duplicate placements (23505),
			// removals of rows another worker already removed
			// (P0002), clash/capacity violations (YKV01).
			if pgErr.Code == "23505" || pgErr.Code == "P0002" ||
				pgErr.Code == "YKV01" {
				continue
			}
		}

		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLockOrderPreventsDeadlock proves the lock-order doctrine in
// 0013 in both directions.
// First the hazard is shown to be real:
// two transactions acquiring the same two student rows in opposing
// orders, synchronized so each holds its first lock before requesting
// its second, deadlock by construction and one dies with 40P01.
// Then the defense: lock_students sorts internally,
// so concurrent calls with opposing caller orders all acquire the
// lowest id first — no acquisition sequence can hold a later row
// while waiting for an earlier one, and no 40P01 can arise.
func TestLockOrderPreventsDeadlock(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)
	seedRace(t, pool, 2, 10)

	ctx := context.Background()

	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = txA.Rollback(ctx) }()

	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = txB.Rollback(ctx) }()

	lock := func(tx interface {
		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	}, id string,
	) error {
		if _, err := tx.Exec(ctx,
			`SELECT 1 FROM students WHERE id = $1 FOR UPDATE`, id); err != nil {
			return fmt.Errorf("lock student %s: %w", id, err)
		}

		return nil
	}

	if err := lock(txA, "s1"); err != nil {
		t.Fatalf("lock s1: %v", err)
	}

	if err := lock(txB, "s2"); err != nil {
		t.Fatalf("lock s2: %v", err)
	}

	// Both hold their first row; now each requests the other's.
	errs := race(t, 2, func(i int) error {
		if i == 1 {
			return lock(txA, "s2")
		}

		return lock(txB, "s1")
	})

	deadlocks := 0

	for _, err := range errs {
		if err == nil {
			continue
		}

		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "40P01" {
			deadlocks++

			continue
		}

		t.Fatalf("unexpected error: %v", err)
	}

	if deadlocks != 1 {
		t.Fatalf("opposing lock orders produced %d deadlocks, want exactly 1", deadlocks)
	}

	_ = txA.Rollback(ctx)
	_ = txB.Rollback(ctx)

	// The same contention through lock_students, with every worker
	// naming the students in a different order: the internal sort
	// makes deadlock structurally impossible, not merely unlikely.
	errs = race(t, 16, func(i int) error {
		ids := []string{"s1", "s2"}
		if i%2 == 0 {
			ids = []string{"s2", "s1"}
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := tx.Exec(ctx,
			`SELECT lock_students($1)`, ids); err != nil {
			return fmt.Errorf("lock_students: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit: %w", err)
		}

		return nil
	})

	for _, err := range errs {
		if err != nil {
			t.Fatalf("sorted locking must never deadlock: %v", err)
		}
	}
}

// TestGradeLockOrderPreventsDeadlock covers the third axis of the
// lock order, the one that is easy to miss because half of it is
// never written down: inserting or updating a student takes a
// FOR KEY SHARE lock on the grade it references, as part of the
// foreign key check.
//
// First the hazard, by construction: a transaction that locks a
// student and then writes its grade_id runs students-then-grades,
// and deadlocks against set_max_budgeted_periods, which runs
// grades-then-students.
//
// Then the defense: upsert_students takes its grade locks up front,
// so every path runs grades-then-students and no cycle can form.
func TestGradeLockOrderPreventsDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()

	exec(t, pool, `INSERT INTO grades
			(id, name, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', 6, 0, 1), ('Y10', 'Year 10', 6, 0, 2);
		INSERT INTO students (id, name, grade_id, legal_sex)
			VALUES ('s1', 'Student One', 'Y9', 'F')`)

	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = txA.Rollback(ctx) }()

	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = txB.Rollback(ctx) }()

	// A holds the student; B holds the grade.
	if _, err := txA.Exec(ctx,
		`SELECT 1 FROM students WHERE id = 's1' FOR UPDATE`); err != nil {
		t.Fatalf("lock student: %v", err)
	}

	if _, err := txB.Exec(ctx,
		`SELECT 1 FROM grades WHERE id = 'Y10' FOR UPDATE`); err != nil {
		t.Fatalf("lock grade: %v", err)
	}

	// A now wants the grade, through the foreign key it never names;
	// B wants the student.
	errs := race(t, 2, func(i int) error {
		if i == 1 {
			_, err := txA.Exec(ctx,
				`UPDATE students SET grade_id = 'Y10' WHERE id = 's1'`)

			return err //nolint:wrapcheck // the SQLSTATE is the subject
		}

		_, err := txB.Exec(ctx,
			`SELECT 1 FROM students WHERE id = 's1' FOR UPDATE`)

		return err //nolint:wrapcheck // the SQLSTATE is the subject
	})

	deadlocks := 0

	for _, err := range errs {
		if err == nil {
			continue
		}

		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "40P01" {
			deadlocks++

			continue
		}

		t.Fatalf("unexpected error: %v", err)
	}

	if deadlocks != 1 {
		t.Fatalf("opposing grade/student orders produced %d deadlocks, want 1",
			deadlocks)
	}

	_ = txA.Rollback(ctx)
	_ = txB.Rollback(ctx)

	// The same contention through the write functions.
	//
	// The reachable shape is an import that both touches a student
	// already in the grade and inserts a new one into it: the first
	// takes the student row lock, the second takes the grade's key
	// lock through the foreign key. Against a concurrent cap change
	// on that grade, which holds the grade and wants its students,
	// that is the cycle above. upsert_students takes its grade locks
	// before any student lock, so the cycle cannot form.
	errs = race(t, 16, func(i int) error {
		if i%2 == 0 {
			return q.UpsertStudents(ctx, db.UpsertStudentsParams{
				PIds:        []string{"s1", fmt.Sprintf("n%d", i)},
				PNames:      []string{"Student One", "New Student"},
				PGradeIds:   []string{"Y9", "Y9"},
				PLegalSexes: []string{"F", "M"},
				PAccept:     []string{},
			})
		}

		return q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
			GradeID:            "Y9",
			MaxBudgetedPeriods: pgtype.Int8{Int64: int64(6 + i), Valid: true},
			Accept:             []string{},
		})
	})

	for _, err := range errs {
		if err != nil {
			t.Fatalf("the grade-first order must never deadlock: %v", err)
		}
	}
}
