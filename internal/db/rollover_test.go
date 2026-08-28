package db_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The end of a season is one action, not one per grade, and it must
// not be destructive: the bounds are a schedule an administrator
// entered by hand, often staggered across grades, and a season ends
// several times over its life — a trial run, a correction, the real
// thing. A close that erased the schedule would make the second
// closing impossible to prepare for.

func at(offset time.Duration) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(offset), Valid: true, InfinityModifier: pgtype.Finite}
}

// window arranges one grade's bounds directly, because the point of
// these tests is what closing does to bounds that are already there.
func window(t *testing.T, q *db.Queries, id string, opens, closes pgtype.Timestamptz) {
	t.Helper()

	ctx := context.Background()
	if err := q.NewGrade(ctx, db.NewGradeParams{ID: id, Name: "Grade " + id}); err != nil {
		t.Fatalf("new grade %s: %v", id, err)
	}

	if _, err := q.SetGradeWindow(ctx, db.SetGradeWindowParams{
		ID: id, SetOpensAt: true, OpensAt: opens, SetClosesAt: true, ClosesAt: closes,
	}); err != nil {
		t.Fatalf("window %s: %v", id, err)
	}
}

func bounds(t *testing.T, q *db.Queries, id string) (opens, closes pgtype.Timestamptz, open bool) {
	t.Helper()

	rows, err := q.GetGrades(context.Background())
	if err != nil {
		t.Fatalf("get grades: %v", err)
	}

	for _, g := range rows {
		if g.ID == id {
			return g.OpensAt, g.ClosesAt, g.IsOpen.Bool
		}
	}

	t.Fatalf("grade %s vanished", id)

	return opens, closes, open
}

func TestClosingEveryWindowKeepsTheSchedulesItDidNotNeedToTouch(t *testing.T) {
	t.Parallel()
	_, q := fresh(t)

	ctx := context.Background()
	// One of each state a grade can be in when somebody presses the
	// button: running, not yet started, and already finished.
	window(t, q, "OPEN", at(-time.Hour), pgtype.Timestamptz{})
	window(t, q, "LATER", at(24*time.Hour), at(48*time.Hour))
	window(t, q, "DONE", at(-48*time.Hour), at(-24*time.Hour))

	laterOpens, laterCloses, _ := bounds(t, q, "LATER")
	doneOpens, doneCloses, _ := bounds(t, q, "DONE")

	closed, err := q.CloseOpenWindows(ctx)
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	// Exactly the one that was open. Naming the others would give them
	// a closing time they never had.
	if closed != 1 {
		t.Errorf("closed %d grades, want 1", closed)
	}

	opens, closes, open := bounds(t, q, "OPEN")
	if open {
		t.Error("OPEN is still open after closing every window")
	}

	if !opens.Valid {
		t.Error("OPEN lost its opens_at; the row no longer records that the window ran")
	}

	if !closes.Valid {
		t.Error("OPEN has no closes_at, so nothing says when it ended")
	}

	// The grade whose window is scheduled for next week is the reason
	// this is not an UPDATE of both bounds to NULL: erasing it is
	// unrecoverable, and nothing about ending today's window says
	// anything about next week's.
	gotOpens, gotCloses, stillOpen := bounds(t, q, "LATER")
	if stillOpen {
		t.Error("LATER reports open, but its window has not started")
	}

	if !gotOpens.Time.Equal(laterOpens.Time) || !gotCloses.Time.Equal(laterCloses.Time) {
		t.Errorf("LATER's schedule moved: %v..%v, want %v..%v",
			gotOpens.Time, gotCloses.Time, laterOpens.Time, laterCloses.Time)
	}

	gotOpens, gotCloses, _ = bounds(t, q, "DONE")
	if !gotOpens.Time.Equal(doneOpens.Time) || !gotCloses.Time.Equal(doneCloses.Time) {
		t.Errorf("DONE's record moved: %v..%v, want %v..%v",
			gotOpens.Time, gotCloses.Time, doneOpens.Time, doneCloses.Time)
	}
}

func TestClosingEveryWindowIsIdempotent(t *testing.T) {
	t.Parallel()
	_, q := fresh(t)

	ctx := context.Background()

	window(t, q, "OPEN", at(-time.Hour), pgtype.Timestamptz{})

	if _, err := q.CloseOpenWindows(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, first, _ := bounds(t, q, "OPEN")

	// The second press is the interesting one: an administrator who is
	// not sure it worked, or two of them at once. It must not move the
	// closing time, or the record of when enrollment ended would drift
	// forward with every click.
	closed, err := q.CloseOpenWindows(ctx)
	if err != nil {
		t.Fatalf("close again: %v", err)
	}

	if closed != 0 {
		t.Errorf("second close touched %d grades, want 0", closed)
	}

	_, second, _ := bounds(t, q, "OPEN")
	if !first.Time.Equal(second.Time) {
		t.Errorf("closes_at moved from %v to %v on a second close", first.Time, second.Time)
	}
}

func TestOpenWindowsAreCountedTheWayOpennessIsRead(t *testing.T) {
	t.Parallel()
	_, q := fresh(t)

	ctx := context.Background()
	// A reset asks this question to decide whether to refuse, so the
	// answer has to agree with what the write functions' gates think —
	// which is why it reads v_grades rather than testing the columns
	// itself.
	window(t, q, "OPEN", at(-time.Hour), at(time.Hour))
	window(t, q, "LATER", at(time.Hour), pgtype.Timestamptz{})
	window(t, q, "NEVER", pgtype.Timestamptz{}, pgtype.Timestamptz{})

	open, err := q.CountOpenWindows(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if open != 1 {
		t.Errorf("count = %d, want 1", open)
	}

	if _, err := q.CloseOpenWindows(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}

	if open, err = q.CountOpenWindows(ctx); err != nil {
		t.Fatalf("count: %v", err)
	} else if open != 0 {
		t.Errorf("count = %d after closing everything, want 0", open)
	}
}

// sort_order is deliberately not unique: creating a period takes
// MAX + 1, and two administrators creating one at the same moment both
// get the same number. The schema says that is harmless because "ties
// are broken by id wherever the order is read" — and two of the places
// it is read, the array aggregates in v_courses, did not do that.
//
// This is asserted against the schema text rather than against a
// result, because a result cannot show it. With ties present the
// planner today returns the rows in insertion order, which is usually
// id order anyway; forcing seqscan, hashjoin and mergejoin all
// produced the same array. What was missing was never the current
// answer. It was the guarantee, and the guarantee is the ORDER BY.
func TestEveryOrderingByASharedSortOrderNamesATiebreak(t *testing.T) {
	t.Parallel()

	// ORDER BY clauses that mention sort_order, wherever they appear:
	// in a view, in an aggregate, in a query file.
	ordering := regexp.MustCompile(`(?i)ORDER BY([^)\n;]*sort_order[^)\n;]*)`)

	for _, dir := range []string{"schemas", "queries"} {
		paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}

		if len(paths) == 0 {
			t.Fatalf("no .sql files under %s; the test is looking in the wrong place", dir)
		}

		for _, path := range paths {
			body, err := os.ReadFile(path) //#nosec G304 -- fixed test data
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			for _, match := range ordering.FindAllStringSubmatch(string(body), -1) {
				clause := match[1]
				// An id is what breaks the tie, because it is the only
				// column that cannot itself be tied.
				if !strings.Contains(clause, "id") {
					t.Errorf("%s: ORDER BY%s has no id to break a tie on", path, clause)
				}
			}
		}
	}
}

// Cancelling one course and resetting every course are not the same
// operation, and the difference is deliberate: the first names a
// course and takes its enrollments with it, the second names nothing
// and is refused while any enrollment still exists.
//
// They sit next to each other on the same page, so this pins which is
// which — and that the refusal is the foreign key's, not a check
// somebody remembered to write.
func TestCancellingOneCourseAndResettingThemAllDifferDeliberately(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	arrange := func(courseID string) {
		t.Helper()

		if err := q.CreateCourse(ctx, db.CreateCourseParams{
			PCourseID: courseID, PName: "Course " + courseID, PDescription: "",
			PCategoryID: "CAT", PTerm: "2026", PMaxStudents: capacity(10),
			PPeriodIds: []string{}, PGradeIds: []string{},
		}); err != nil {
			t.Fatalf("create course %s: %v", courseID, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
			 VALUES ('s1', $1, true, true)`, courseID); err != nil {
			t.Fatalf("enroll into %s: %v", courseID, err)
		}
	}

	if err := q.NewCategory(ctx, db.NewCategoryParams{ID: "CAT", Name: "Category"}); err != nil {
		t.Fatalf("new category: %v", err)
	}

	if err := q.NewGrade(ctx, db.NewGradeParams{ID: "Y9", Name: "Year 9"}); err != nil {
		t.Fatalf("new grade: %v", err)
	}

	// Straight in, as elsewhere in this package: the roster importer is
	// not what is under test, and going through it would put a second
	// thing in front of the first.
	if _, err := pool.Exec(ctx,
		`INSERT INTO students (id, name, grade_id, legal_sex)
		 VALUES ('s1', 'Student', 'Y9', 'F')`); err != nil {
		t.Fatalf("new student: %v", err)
	}

	arrange("C1")
	arrange("C2")

	// Naming one is a decision about that one, and it takes the
	// enrollment with it.
	if err := q.DeleteCourse(ctx, "C1"); err != nil {
		t.Fatalf("delete_course: %v", err)
	}

	var held int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM enrollments WHERE course_id = 'C1'`).Scan(&held); err != nil {
		t.Fatalf("count: %v", err)
	}

	if held != 0 {
		t.Errorf("%d enrollments survived the course being cancelled", held)
	}

	// Naming none is a sweep, and a sweep run before the enrollments
	// were cleared is a sweep run in the wrong order.
	err := q.DeleteAllCourses(ctx)
	if err == nil {
		t.Fatal("resetting the courses removed enrollments nobody asked about")
	}

	if got := pgCode(err); got != "23503" {
		t.Errorf("SQLSTATE = %q, want 23503; the refusal should be the "+
			"foreign key's rather than something hand-written", got)
	}

	// And once they are cleared, in the order the refusal asked for.
	if err := q.DeleteAllEnrollments(ctx); err != nil {
		t.Fatalf("clear enrollments: %v", err)
	}

	if err := q.DeleteAllCourses(ctx); err != nil {
		t.Fatalf("reset courses after clearing enrollments: %v", err)
	}
}
