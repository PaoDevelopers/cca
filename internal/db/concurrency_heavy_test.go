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

// Concurrency at the scale the system actually meets: a window opening
// on a full school, and every write shape in the vocabulary running at
// once against the same rows.
//
// These are the tests that cannot be replaced by reasoning. The lock
// order is documented and the checks are inside the lock, but both
// claims are about what PostgreSQL does with real sessions, and the
// failure modes — a seat sold twice, a budget exceeded, a deadlock —
// are all invisible to a serial test.

// pgCode returns the SQLSTATE of err, or "" if it is not a database
// error.
func pgCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}

	return ""
}

// Turbulence a mixed race is expected to produce: rows another worker
// got to first, and violations of the rules being contended.
func expectedUnderContention(code string) bool {
	switch code {
	case "23505", // already enrolled
		"23503", // a row it names was deleted by another worker
		"P0002", // the row a removal named is already gone
		"YKV01", // a contended rule
		"YKG01", // the window closed under a student
		"YKG03": // not theirs to drop
		return true
	}

	return false
}

// demandNoDeadlock fails the test on 40P01 or 40001, and on anything
// the race was not designed to produce. Serialization failures are
// included: nothing here runs at an isolation level that can raise
// one, so if one appears the lock doctrine has been left behind.
func demandNoDeadlock(t *testing.T, errs []error, what string) {
	t.Helper()

	for _, err := range errs {
		if err == nil {
			continue
		}

		switch code := pgCode(err); {
		case code == "40P01":
			t.Fatalf("%s: deadlock: %v", what, err)
		case code == "40001":
			t.Fatalf("%s: serialization failure: %v", what, err)
		case expectedUnderContention(code):
		default:
			t.Fatalf("%s: unexpected error: %v", what, err)
		}
	}
}

// The whole write vocabulary against the same rows at once, including
// the three functions the application layer added after the original
// concurrency suite was written. If any pair of them disagrees about
// the lock order, this is where it shows.
func TestEveryWriteShapeTogetherDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 8
	seedRace(t, pool, students, 1000)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order)
			VALUES ('TUE1', 'Tuesday', 2);
		INSERT INTO categories (id, name) VALUES ('ART', 'Art');
		INSERT INTO grades (id, name, opens_at, closes_at,
				max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y10', 'Year 10', now() - interval '1 hour', NULL, NULL, 0, 2)`)

	ids := make([]string, students)
	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
	}

	const shapes = 8

	errs := race(t, shapes*students, func(i int) error {
		ctx := context.Background()

		switch i % shapes {
		case 0:
			return q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
				PCourseID:   "RACE",
				PStudentIds: ids,
				PAccept:     []string{},
			})
		case 1:
			return q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
				PCourseID: "RACE", PStudentIds: ids[:students/2],
			})
		case 2:
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1", "TUE1"}, 500))
		case 3:
			return q.SetEnrollmentPolicy(ctx, db.SetEnrollmentPolicyParams{
				PCourseID: "RACE", PStudentIds: ids[:1],
				PStudentDroppable: true, PCountsTowardBudget: false,
				PAccept: []string{},
			})
		case 4:
			// Moves students between grades, which is the write that
			// takes grade locks on its way to student rows.
			grade := "Y9"
			if i%2 == 0 {
				grade = "Y10"
			}

			return q.UpsertStudents(ctx, db.UpsertStudentsParams{
				PIds:        ids[:2],
				PNames:      []string{"One", "Two"},
				PGradeIds:   []string{grade, grade},
				PLegalSexes: []string{"F", "F"},
				PAccept:     []string{},
			})
		case 5:
			return q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
				GradeID: "Y9", Accept: []string{},
			})
		case 6:
			// The course batch, which touches courses, grades and
			// students in one call.
			return upsertCourses(q, []courseSpec{
				course("RACE", courseSpec{
					"name": "Contested", "period_ids": []string{"MON1"},
					"max_students": 1000,
					"grade_ids":    []string{"Y9", "Y10"},
				}),
			})
		default:
			return q.SelfEnroll(ctx, db.SelfEnrollParams{
				PStudentID: ids[i%students], PCourseID: "RACE",
			})
		}
	})

	demandNoDeadlock(t, errs, "mixed write shapes")
}

// A student's budget is a sum over all their enrollments, so it is the
// one rule a per-row lock does not obviously protect: two sessions
// enrolling the same student into two different courses each read a
// total that does not yet include the other. The student row lock is
// what serializes them, and this is the test that says so.
func TestBudgetIsNotExceededUnderConcurrentSelfEnrollment(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const courses = 12

	seedRace(t, pool, 1, 1000)
	// One period each, and a budget of three: nine of the twelve
	// attempts must lose.
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 3 WHERE id = 'Y9'`)

	for i := range courses {
		id := fmt.Sprintf("C%d", i)
		exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
				invite_only, teacher, teacher_email, location, term, cost, category_id)
			VALUES ($1, $2, '', 100, FALSE, '', '', '', 'Season', '', 'SPORT')`, id, id)
		exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ($1, $2, $3)`,
			fmt.Sprintf("P%d", i), fmt.Sprintf("P%d", i), i+2)
		exec(t, pool, `INSERT INTO course_periods (course_id, period_id) VALUES ($1, $2)`,
			id, fmt.Sprintf("P%d", i))
	}

	errs := race(t, courses, func(i int) error {
		return q.SelfEnroll(context.Background(), db.SelfEnrollParams{
			PStudentID: "s1", PCourseID: fmt.Sprintf("C%d", i-1),
		})
	})

	demandNoDeadlock(t, errs, "budget race")

	// The invariant: the sum the rule guards was never exceeded, no
	// matter how the twelve interleaved.
	used := count(t, pool, `SELECT count(*)
		FROM enrollments e
		JOIN course_periods cp ON cp.course_id = e.course_id
		WHERE e.student_id = 's1' AND e.counts_toward_budget`)
	if used > 3 {
		t.Fatalf("budget exceeded under contention: %d periods of 3", used)
	}

	// And it was actually filled: a race that admitted nobody would
	// pass the check above while proving nothing.
	if used != 3 {
		t.Fatalf("budget under-filled: %d periods of 3", used)
	}
}

// The clash rule has the same shape as the budget one — it reads the
// student's other enrollments — so it has the same hazard. Two
// sessions enrolling one student into two courses that share a period
// must not both succeed.
func TestClashHoldsUnderConcurrentSelfEnrollment(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const rivals = 10

	seedRace(t, pool, 1, 1000)

	// Every course meets in MON1, so any two of them clash.
	for i := range rivals {
		id := fmt.Sprintf("C%d", i)
		exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
				invite_only, teacher, teacher_email, location, term, cost, category_id)
			VALUES ($1, $2, '', 100, FALSE, '', '', '', 'Season', '', 'SPORT')`, id, id)
		exec(t, pool, `INSERT INTO course_periods (course_id, period_id)
			VALUES ($1, 'MON1')`, id)
	}

	errs := race(t, rivals, func(i int) error {
		return q.SelfEnroll(context.Background(), db.SelfEnrollParams{
			PStudentID: "s1", PCourseID: fmt.Sprintf("C%d", i-1),
		})
	})

	demandNoDeadlock(t, errs, "clash race")

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE student_id = 's1'`); n != 1 {
		t.Fatalf("%d clashing enrollments were admitted, want exactly 1", n)
	}
}

// The window is read inside the lock, so closing it while a rush is in
// progress must stop admissions at a definite point rather than
// letting some through behind the close.
func TestClosingTheWindowMidRushAdmitsNobodyAfterwards(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 24

	seedRace(t, pool, students, 1000)

	var wg sync.WaitGroup

	errs := make([]error, students)

	for i := range students {
		wg.Go(func() {
			errs[i] = q.SelfEnroll(context.Background(), db.SelfEnrollParams{
				PStudentID: fmt.Sprintf("s%d", i+1), PCourseID: "RACE",
			})
		})
	}

	// Slams shut underneath them.
	wg.Go(func() {
		exec(t, pool, `UPDATE grades SET closes_at = now() WHERE id = 'Y9'`)
	})

	wg.Wait()
	demandNoDeadlock(t, errs, "closing window")

	// Whoever was refused was refused for the window, and whoever got
	// in got in before it shut: the count and the successes agree.
	var admitted int

	for _, err := range errs {
		if err == nil {
			admitted++
		} else if code := pgCode(err); code != "YKG01" {
			t.Fatalf("a refusal during the close was %s, not the window: %v", code, err)
		}
	}

	if n := int(count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`)); n != admitted {
		t.Fatalf("%d enrollments for %d successful calls", n, admitted)
	}

	// And the window really is shut now, so a late arrival is refused
	// — by the window, and by nothing else.
	//
	// The late arrival is a student who took no part in the rush. It
	// used to be s1, who did, so the answer was the duplicate key from
	// their own successful enrollment above, and 23505 was accepted
	// alongside YKG01. That accepted answer is the one a removed
	// window gate also gives, so the check could not fail.
	exec(t, pool, `INSERT INTO students (id, name, grade_id, legal_sex)
		VALUES ('latecomer', 'Late Comer', 'Y9', 'X')`)

	err := q.SelfEnroll(context.Background(), db.SelfEnrollParams{
		PStudentID: "latecomer", PCourseID: "RACE",
	})
	if code := pgCode(err); code != "YKG01" {
		t.Fatalf("after the close, enrollment gave %s, want YKG01: %v", code, err)
	}
}

// A swap is atomic or it is nothing. Racing a whole grade through the
// same swap must never leave a student holding neither course, which
// is what a drop-then-add would do under contention for the new seat.
func TestSwapIsAtomicUnderContention(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 16

	// One seat in the destination: fifteen swaps must fail, and fail
	// without having dropped anything.
	seedRace(t, pool, students, 1000)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ('TUE1', 'Tuesday', 2);
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id)
			VALUES ('DEST', 'Destination', '', 1, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES ('DEST', 'MON1')`)

	for i := range students {
		exec(t, pool, `INSERT INTO enrollments
			(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ($1, 'RACE', TRUE, TRUE)`, fmt.Sprintf("s%d", i+1))
	}

	errs := race(t, students, func(i int) error {
		return q.SelfSwap(context.Background(), db.SelfSwapParams{
			PStudentID:    fmt.Sprintf("s%d", i),
			POldCourseIds: []string{"RACE"},
			PCourseID:     "DEST",
		})
	})

	demandNoDeadlock(t, errs, "swap race")

	// Exactly one seat was sold.
	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'DEST'`); n != 1 {
		t.Fatalf("DEST holds %d, want 1", n)
	}

	// And nobody fell between the two: every student holds exactly one
	// course, the old one or the new.
	if n := count(t, pool, `SELECT count(*) FROM students s
		WHERE (SELECT count(*) FROM enrollments e WHERE e.student_id = s.id) <> 1`); n != 0 {
		t.Fatalf("%d student(s) hold neither or both courses after a swap race", n)
	}
}

// Placement is an ordered batch: students compete for the same seats
// and the ones named first win when the course fills. Racing two
// batches that disagree about the order must still leave the course at
// its capacity and no further.
func TestPlacementBatchesNeverOversell(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const (
		students = 20
		seats    = 6
	)

	seedRace(t, pool, students, seats)

	ids := make([]string, students)
	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
	}

	reversed := make([]string, students)
	for i, id := range ids {
		reversed[students-1-i] = id
	}

	errs := race(t, 12, func(i int) error {
		order := ids
		if i%2 == 0 {
			order = reversed
		}

		return q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
			PCourseID: "RACE", PStudentIds: order, PAccept: []string{},
		})
	})

	demandNoDeadlock(t, errs, "placement race")

	// Every batch here exceeds the capacity, so every batch must have
	// been refused whole: the course is untouched.
	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`); n != 0 {
		t.Fatalf("a refused placement batch left %d row(s) behind", n)
	}

	// Refused for the capacity, and not for something incidental. A
	// race where every batch happened to fail on a duplicate key would
	// leave the course untouched too, and would say nothing at all
	// about overselling.
	var sawCapacity bool

	for _, err := range errs {
		if err == nil {
			t.Fatal("a batch of 20 into 6 seats was accepted")
		}

		if code := pgCode(err); code == "YKV01" {
			sawCapacity = true
		}
	}

	if !sawCapacity {
		t.Fatal("no batch was refused for the capacity; the race proved nothing")
	}

	// And the seats are genuinely available to a batch that fits, so
	// the course was not simply broken for everyone.
	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "RACE", PStudentIds: ids[:seats], PAccept: []string{},
	}); err != nil {
		t.Fatalf("a batch that fits was refused: %v", err)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM enrollments WHERE course_id = 'RACE'`); n != seats {
		t.Fatalf("the fitting batch placed %d of %d", n, seats)
	}
}

// The shape of a real window opening: a full school's worth of
// students, each taking several courses at once, against a catalogue
// with contended seats. Nothing may deadlock, nothing may oversell,
// and no course may end up over its capacity.
func TestWindowOpeningStorm(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const (
		students = 120
		courses  = 20
		seats    = 6
		periods  = 8
	)

	seedRace(t, pool, students, 1000)

	for i := range periods {
		exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ($1, $2, $3)`,
			fmt.Sprintf("P%d", i), fmt.Sprintf("P%d", i), i+2)
	}

	for i := range courses {
		id := fmt.Sprintf("C%d", i)
		exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
				invite_only, teacher, teacher_email, location, term, cost, category_id)
			VALUES ($1, $2, '', $3, FALSE, '', '', '', 'Season', '', 'SPORT')`, id, id, seats)
		exec(t, pool, `INSERT INTO course_periods (course_id, period_id) VALUES ($1, $2)`,
			id, fmt.Sprintf("P%d", i%periods))
	}

	// Each student rushes three courses, staggered so they collide
	// with different rivals.
	errs := race(t, students*3, func(i int) error {
		student := fmt.Sprintf("s%d", (i-1)/3+1)
		course := fmt.Sprintf("C%d", (i*7)%courses)

		return q.SelfEnroll(context.Background(), db.SelfEnrollParams{
			PStudentID: student, PCourseID: course,
		})
	})

	demandNoDeadlock(t, errs, "window storm")

	// No course sold a seat it did not have.
	over := count(t, pool, `SELECT count(*) FROM v_courses
		WHERE current_students > max_students`)
	if over != 0 {
		t.Fatalf("%d course(s) are over capacity", over)
	}

	// The seats really were contended: a storm that filled nothing
	// would satisfy the check above and prove nothing.
	filled := count(t, pool, `SELECT count(*) FROM v_courses
		WHERE current_students = max_students`)
	if filled == 0 {
		t.Fatal("no course filled; the storm did not contend for anything")
	}

	// And nobody holds two courses in one period.
	clashes := count(t, pool, `SELECT count(*) FROM (
		SELECT e.student_id, cp.period_id
		FROM enrollments e
		JOIN course_periods cp ON cp.course_id = e.course_id
		GROUP BY e.student_id, cp.period_id
		HAVING count(*) > 1) x`)
	if clashes != 0 {
		t.Fatalf("%d student-period cell(s) hold more than one course", clashes)
	}
}

// The course batch against the grade cap, which is the pair that runs
// closest to the lock order's third axis: upsert_courses writes
// course_allowed_grades, taking a foreign key lock on grades on its
// way to the student rows it re-judges, while set_max_budgeted_periods
// runs grades then students. Only taking the grade locks up front
// keeps them in one order.
func TestCourseBatchAgainstGradeCapDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 12

	seedRace(t, pool, students, 1000)
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 10 WHERE id = 'Y9'`)

	for i := range students {
		exec(t, pool, `INSERT INTO enrollments
			(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ($1, 'RACE', TRUE, TRUE)`, fmt.Sprintf("s%d", i+1))
	}

	errs := race(t, 24, func(i int) error {
		if i%2 == 0 {
			return q.SetMaxBudgetedPeriods(context.Background(),
				db.SetMaxBudgetedPeriodsParams{
					GradeID:            "Y9",
					MaxBudgetedPeriods: pgInt8(int64(10 + i)),
					Accept:             []string{},
				})
		}

		return upsertCourses(q, []courseSpec{
			course("RACE", courseSpec{
				"name": "Contested", "period_ids": []string{"MON1"},
				"max_students": 1000,
				"grade_ids":    []string{"Y9"},
			}),
		})
	})

	demandNoDeadlock(t, errs, "course batch against grade cap")
}

// The same pair, with update_course in the batch's place. It writes
// course_allowed_grades too, so it takes the same implicit grade lock
// — and it must take it in the same order, or these two deadlock by
// construction.
func TestUpdateCourseAgainstGradeCapDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 12

	seedRace(t, pool, students, 1000)
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 10 WHERE id = 'Y9'`)

	for i := range students {
		exec(t, pool, `INSERT INTO enrollments
			(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ($1, 'RACE', TRUE, TRUE)`, fmt.Sprintf("s%d", i+1))
	}

	restricted := raceCourse([]string{"MON1"}, 1000)
	restricted.PGradeIds = []string{"Y9"}

	errs := race(t, 24, func(i int) error {
		if i%2 == 0 {
			return q.SetMaxBudgetedPeriods(context.Background(),
				db.SetMaxBudgetedPeriodsParams{
					GradeID:            "Y9",
					MaxBudgetedPeriods: pgInt8(int64(10 + i)),
					Accept:             []string{},
				})
		}

		return q.UpdateCourse(context.Background(), restricted)
	})

	demandNoDeadlock(t, errs, "update_course against grade cap")
}

// Two administrators re-importing the same spreadsheet in opposite
// orders. Each call locks its courses ascending over the whole batch,
// so the file's own order cannot become a lock order.
func TestOpposingCourseImportsDoNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 2, 100)

	const catalogue = 12

	forward := make([]courseSpec, catalogue)
	backward := make([]courseSpec, catalogue)

	for i := range catalogue {
		spec := course(fmt.Sprintf("C%02d", i), courseSpec{
			"period_ids": []string{"MON1"},
		})
		forward[i] = spec
		backward[catalogue-1-i] = spec
	}

	errs := race(t, 16, func(i int) error {
		if i%2 == 0 {
			return upsertCourses(q, forward)
		}

		return upsertCourses(q, backward)
	})

	demandNoDeadlock(t, errs, "opposing course imports")

	if n := count(t, pool, `SELECT count(*) FROM courses`); n != catalogue+1 {
		t.Fatalf("courses = %d, want %d", n, catalogue+1)
	}
}

// Roster imports and course imports at once: the two batch writes,
// which between them touch every table the lock order names.
func TestOpposingBatchWritesDoNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 10

	seedRace(t, pool, students, 1000)
	exec(t, pool, `INSERT INTO grades (id, name, opens_at, closes_at,
			max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('Y10', 'Year 10', now() - interval '1 hour', NULL, NULL, 0, 2)`)

	ids := make([]string, students)
	names := make([]string, students)
	sexes := make([]string, students)

	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
		names[i] = fmt.Sprintf("Student %d", i+1)
		sexes[i] = "F"
	}

	errs := race(t, 20, func(i int) error {
		if i%2 == 0 {
			grades := make([]string, students)
			for j := range grades {
				if i%4 == 0 {
					grades[j] = "Y9"
				} else {
					grades[j] = "Y10"
				}
			}

			return q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
				PIds: ids, PNames: names, PGradeIds: grades,
				PLegalSexes: sexes, PAccept: []string{},
			})
		}

		return upsertCourses(q, []courseSpec{
			course("RACE", courseSpec{
				"name": "Contested", "period_ids": []string{"MON1"},
				"max_students": 1000,
				"grade_ids":    []string{"Y9", "Y10"},
			}),
		})
	})

	demandNoDeadlock(t, errs, "opposing batch writes")
}

// A student acting in two tabs at once, which is the commonest real
// concurrency in the system and the one nobody thinks of as
// concurrency at all.
func TestOneStudentInTwoTabs(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 1, 1000)

	// OTHER meets in the same period as RACE, which is the whole
	// point: the two courses clash, so "not double-booked" is a fact
	// the clash rule has to establish rather than one the fixture
	// hands over. It used to be scheduled in a period of its own,
	// where nothing the student did could have double-booked them and
	// the assertion below held whatever the rules did.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
			invite_only, teacher, teacher_email, location, term, cost,
			category_id)
		VALUES ('OTHER', 'Other', '', 100, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id)
			VALUES ('OTHER', 'MON1')`)

	// Enroll, drop, swap and enroll again, all at once, from the same
	// student.
	errs := race(t, 40, func(i int) error {
		ctx := context.Background()

		switch i % 4 {
		case 0:
			return q.SelfEnroll(ctx, db.SelfEnrollParams{
				PStudentID: "s1", PCourseID: "RACE",
			})
		case 1:
			return q.SelfDrop(ctx, db.SelfDropParams{
				PStudentID: "s1", PCourseID: "RACE",
			})
		case 2:
			return q.SelfEnroll(ctx, db.SelfEnrollParams{
				PStudentID: "s1", PCourseID: "OTHER",
			})
		default:
			return q.SelfSwap(ctx, db.SelfSwapParams{
				PStudentID:    "s1",
				POldCourseIds: []string{"OTHER"},
				PCourseID:     "RACE",
			})
		}
	})

	demandNoDeadlock(t, errs, "one student, two tabs")

	// Whatever the interleaving settled on, it is a legal state.
	//
	// Both courses meet in MON1, so the student may hold one of them
	// or neither, and never both — which is a fact about the clash
	// rule surviving forty concurrent writes from one session, not
	// about the timetable.
	if n := count(t, pool, `SELECT count(*) FROM (
		SELECT cp.period_id
		FROM enrollments e
		JOIN course_periods cp ON cp.course_id = e.course_id
		WHERE e.student_id = 's1'
		GROUP BY cp.period_id
		HAVING count(*) > 1) x`); n != 0 {
		t.Fatalf("the student ended up double-booked in %d period(s)", n)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM enrollments WHERE student_id = 's1'`); n > 1 {
		t.Fatalf("the student holds %d clashing enrollments, want at most 1", n)
	}

	// And every refusal was one of the answers this race can produce.
	// A refusal for some other reason would mean the writes were
	// failing for a reason the test is not about, and the state
	// assertions above would be holding for the wrong reason.
	for _, err := range errs {
		if err == nil {
			continue
		}

		switch code := pgCode(err); code {
		case "YKV01", "YKG03", "23505", "P0002":
		default:
			t.Fatalf("a concurrent self-write failed with %s: %v", code, err)
		}
	}
}

// A rename cascades through every referencing table, so it holds wide
// locks; it must still take them in the one order.
func TestRenamesDoNotDeadlockAgainstWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 8

	seedRace(t, pool, students, 1000)

	ids := make([]string, students)
	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
	}

	errs := race(t, 16, func(i int) error {
		ctx := context.Background()

		switch i % 4 {
		case 0:
			return q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
				PCourseID: "RACE", PStudentIds: ids, PAccept: []string{},
			})
		case 1:
			return q.RemoveEnrollments(ctx, db.RemoveEnrollmentsParams{
				PCourseID: "RACE", PStudentIds: ids,
			})
		case 2:
			if _, err := q.RenameCategory(ctx, db.RenameCategoryParams{
				ID: "SPORT", Name: fmt.Sprintf("Sports %d", i),
			}); err != nil {
				return fmt.Errorf("rename_category: %w", err)
			}

			return nil
		default:
			if _, err := q.UpdateGradeSettings(ctx, db.UpdateGradeSettingsParams{
				ID: "Y9", Name: fmt.Sprintf("Year 9 (%d)", i),
			}); err != nil {
				return fmt.Errorf("update_grade_settings: %w", err)
			}

			return nil
		}
	})

	demandNoDeadlock(t, errs, "renames against writes")
}

// The eligibility read runs on every catalogue load, so it runs
// constantly while writes are landing. It must never see a torn state
// — a clash against a course that no longer exists, say — and must
// never block a write into a deadlock.
func TestEligibilityReadUnderWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	const students = 6

	seedRace(t, pool, students, 1000)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order) VALUES ('TUE1', 'Tuesday', 2)`)

	ids := make([]string, students)
	for i := range students {
		ids[i] = fmt.Sprintf("s%d", i+1)
	}

	errs := race(t, 36, func(i int) error {
		ctx := context.Background()

		switch i % 3 {
		case 0:
			_, err := q.StudentCourseViolations(ctx, db.StudentCourseViolationsParams{
				PStudentID: ids[i%students], PCountsTowardBudget: true,
			})
			if err != nil {
				return fmt.Errorf("student_course_violations: %w", err)
			}

			return nil
		case 1:
			return q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
				PCourseID: "RACE", PStudentIds: ids, PAccept: []string{},
			})
		default:
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1", "TUE1"}, 500))
		}
	})

	demandNoDeadlock(t, errs, "eligibility read under writes")
}

// Both whole-catalogue reads while the catalogue is being rewritten
// under them. A read model that joined inconsistently would surface
// here as an error rather than a wrong answer, which is the only part
// a test can insist on cheaply.
func TestWholeCatalogueReadsUnderRewrite(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 4, 1000)

	for i := range 6 {
		exec(t, pool, `INSERT INTO enrollments
			(student_id, course_id, student_droppable, counts_toward_budget)
			VALUES ($1, 'RACE', TRUE, TRUE) ON CONFLICT DO NOTHING`,
			fmt.Sprintf("s%d", i%4+1))
	}

	errs := race(t, 30, func(i int) error {
		ctx := context.Background()

		switch i % 5 {
		case 0:
			_, err := q.GetCourses(ctx)
			if err != nil {
				return fmt.Errorf("GetCourses: %w", err)
			}

			return nil
		case 1:
			_, err := q.GetEnrollmentsExport(ctx)
			if err != nil {
				return fmt.Errorf("GetEnrollmentsExport: %w", err)
			}

			return nil
		case 2:
			_, err := q.GetStudentStatus(ctx)
			if err != nil {
				return fmt.Errorf("GetStudentStatus: %w", err)
			}

			return nil
		case 3:
			_, err := q.GetStudentRequirements(ctx)
			if err != nil {
				return fmt.Errorf("GetStudentRequirements: %w", err)
			}

			return nil
		default:
			return q.UpdateCourse(ctx, raceCourse([]string{"MON1"}, int64(100+i)))
		}
	})

	demandNoDeadlock(t, errs, "catalogue reads under rewrite")
}

// pgInt8 is the nullable bigint the generated parameters take.
func pgInt8(n int64) pgtype.Int8 {
	//exhaustruct:ignore
	return pgtype.Int8{Int64: n, Valid: true}
}

// Two administrators loading the same roster in opposite orders. The
// hazard is the one a lock-the-existing-rows pass cannot cover: a
// student the file creates has no row to lock yet, so the insert
// itself is the first lock on it, and two transactions inserting the
// same ids in opposite orders hold each other's next row.
//
// The defense is to walk the batch in a deterministic order rather
// than the file's. Elements of a roster are independent — no student
// competes with another for anything — so the order is free to be
// imposed, and imposing one is what removes the hazard.
func TestOpposingRosterImportsDoNotDeadlock(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 0, 100)

	const roster = 16

	ids := make([]string, roster)
	names := make([]string, roster)
	grades := make([]string, roster)
	sexes := make([]string, roster)

	for i := range roster {
		ids[i] = fmt.Sprintf("n%02d", i)
		names[i] = fmt.Sprintf("New %d", i)
		grades[i] = "Y9"
		sexes[i] = "F"
	}

	reverse := func(in []string) []string {
		out := make([]string, len(in))
		for i, v := range in {
			out[len(in)-1-i] = v
		}

		return out
	}

	backwardIDs := reverse(ids)
	backwardNames := reverse(names)

	errs := race(t, 16, func(i int) error {
		if i%2 == 0 {
			return q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
				PIds: ids, PNames: names, PGradeIds: grades,
				PLegalSexes: sexes, PAccept: []string{},
			})
		}

		return q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
			PIds: backwardIDs, PNames: backwardNames, PGradeIds: grades,
			PLegalSexes: sexes, PAccept: []string{},
		})
	})

	demandNoDeadlock(t, errs, "opposing roster imports")

	if n := count(t, pool, `SELECT count(*) FROM students`); n != roster {
		t.Fatalf("students = %d, want %d", n, roster)
	}
}

// The vocabulary axis of the lock order: deleting a category or a
// period holds the referenced row and then proves RESTRICT against its
// referrers, so any course write that touched those tables after the
// course row would meet it head on. This is the shape that has to be
// hammered rather than reasoned about — it reproduced only under
// repetition.
func TestVocabularyDeletionDoesNotDeadlockAgainstCourseWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 4, 100)
	exec(t, pool, `INSERT INTO periods (id, name, sort_order)
			VALUES ('SPARE', 'Spare', 8), ('KEEP', 'Keep', 9);
		INSERT INTO categories (id, name) VALUES ('SPAREC', 'Spare category')`)

	restricted := raceCourse([]string{"MON1", "SPARE", "KEEP"}, 100)
	restricted.PGradeIds = []string{"Y9"}

	errs := race(t, 36, func(i int) error {
		ctx := context.Background()

		switch i % 6 {
		case 0, 1:
			return q.UpdateCourse(ctx, restricted)
		case 2:
			_, err := q.DeletePeriod(ctx, "SPARE")
			if err != nil {
				return fmt.Errorf("delete_period: %w", err)
			}

			return nil
		case 3:
			_, err := q.DeleteCategory(ctx, "SPAREC")
			if err != nil {
				return fmt.Errorf("delete_category: %w", err)
			}

			return nil
		case 4:
			return upsertCourses(q, []courseSpec{
				course("RACE", courseSpec{
					"name": "Contested", "period_ids": []string{"MON1", "KEEP"},
					"max_students": 100, "grade_ids": []string{"Y9"},
				}),
			})
		default:
			return q.CreateCourse(ctx, db.CreateCourseParams{
				PCourseID: fmt.Sprintf("NEW%02d", i), PName: "New",
				PCategoryID: "SPORT", PTerm: "Season", PMaxStudents: 10,
				PPeriodIds: []string{"MON1", "KEEP"},
				PGradeIds:  []string{"Y9"},
			})
		}
	})

	demandNoDeadlock(t, errs, "vocabulary deletion against course writes")
}

// A grade deletion is the same shape on the third axis: it holds the
// grade and proves RESTRICT against students and course_allowed_grades.
func TestGradeDeletionDoesNotDeadlockAgainstCourseWrites(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	seedRace(t, pool, 4, 100)
	exec(t, pool, `INSERT INTO grades (id, name, min_distinct_categories, sort_order)
		VALUES ('SPAREG', 'Spare grade', 0, 9)`)

	restricted := raceCourse([]string{"MON1"}, 100)
	restricted.PGradeIds = []string{"Y9", "SPAREG"}

	errs := race(t, 24, func(i int) error {
		ctx := context.Background()

		if i%2 == 0 {
			return q.UpdateCourse(ctx, restricted)
		}

		_, err := q.DeleteGrade(ctx, "SPAREG")
		if err != nil {
			return fmt.Errorf("delete_grade: %w", err)
		}

		return nil
	})

	demandNoDeadlock(t, errs, "grade deletion against course writes")
}
