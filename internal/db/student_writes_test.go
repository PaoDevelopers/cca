package db_test

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// upsert_students: insert/update semantics, changed-field re-judge
// scoping, collect-all malformed reporting (YKD01), batch atomicity.

// malformed is one element of a YKD01 DETAIL payload.
type malformed struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	SQLState string `json:"sqlstate"`
	Message  string `json:"message"`
}

// expectMalformed demands err be YKD01 reporting exactly these
// element indices.
func expectMalformed(t *testing.T, err error, indices ...int) []malformed {
	t.Helper()

	pgErr := pgError(t, err, "YKD01")

	var ms []malformed
	if err := json.Unmarshal([]byte(pgErr.Detail), &ms); err != nil {
		t.Fatalf("decode DETAIL %q: %v", pgErr.Detail, err)
	}

	got := make([]int, 0, len(ms))
	for _, m := range ms {
		got = append(got, m.Index)
	}

	slices.Sort(got)

	want := slices.Clone(indices)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("malformed indices = %v, want %v", got, want)
	}

	return ms
}

func seedStudentWrites(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	// Y10's tight cap makes grade moves able to break budget.
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', 4, 0, 1),
			('Y10', 'Year 10', 1, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2);
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('TWICE', 'Two periods', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('Y9ONLY', 'Year 9 only', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('GIRLS', 'Girls only', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1');
		INSERT INTO course_allowed_grades (course_id, grade_id) VALUES ('Y9ONLY', 'Y9');
		INSERT INTO course_allowed_legal_sexes (course_id, legal_sex) VALUES ('GIRLS', 'F')`)
}

func upsert(q *db.Queries, ids, names, grades, sexes []string, accept ...string) error {
	if err := q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
		PIds: ids, PNames: names, PGradeIds: grades, PLegalSexes: sexes,
		PAccept: accept,
	}); err != nil {
		return fmt.Errorf("upsert_students: %w", err)
	}

	return nil
}

func TestUpsertInsertAndUpdate(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedStudentWrites(t, pool)

	// Fresh inserts.
	if err := upsert(q, []string{"s1", "s2"},
		[]string{"Student One", "Student Two"},
		[]string{"Y9", "Y9"}, []string{"F", "M"}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM students`); n != 2 {
		t.Fatalf("fresh elements must insert; %d rows", n)
	}

	// Identical re-import: no violations, nothing to accept.
	if err := upsert(q, []string{"s1", "s2"},
		[]string{"Student One", "Student Two"},
		[]string{"Y9", "Y9"}, []string{"F", "M"}); err != nil {
		t.Fatalf("identical re-import: %v", err)
	}

	// A name fix updates in place.
	if err := upsert(q, []string{"s1"}, []string{"Student One Renamed"},
		[]string{"Y9"}, []string{"F"}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM students
		WHERE id = 's1' AND name = 'Student One Renamed'`); n != 1 {
		t.Fatal("an existing element must update")
	}
}

func TestUpsertChangedFieldScoping(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedStudentWrites(t, pool)

	ctx := context.Background()

	if err := upsert(q, []string{"s1", "s2"},
		[]string{"Student One", "Student Two"},
		[]string{"Y9", "Y9"}, []string{"F", "M"}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Standing accepted sins: s1 in Y9ONLY, GIRLS, and TWICE;
	// s2 (M) in GIRLS with an accepted sex violation.
	place := func(course, student string, charging bool, accept ...string) {
		t.Helper()

		if err := q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: []string{student},
			PStudentDroppable: true, PCountsTowardBudget: charging,
			PAccept: accept,
		}); err != nil {
			t.Fatalf("place %s/%s: %v", course, student, err)
		}
	}
	place("Y9ONLY", "s1", false)
	place("GIRLS", "s1", false)
	place("GIRLS", "s2", false, "legal_sex:s2:GIRLS")
	place("TWICE", "s1", true)

	// An unchanged re-import must not resurface s2's accepted sin,
	// and a pure name change must not either.
	if err := upsert(q, []string{"s1", "s2"},
		[]string{"Student One", "Student Two B"},
		[]string{"Y9", "Y9"}, []string{"F", "M"}); err != nil {
		t.Fatalf("re-import resurfaced accepted placements: %v", err)
	}

	// A grade move re-judges grade and budget, nothing else:
	// s1 to Y10 violates Y9ONLY's restriction and Y10's cap of 1
	// (TWICE charges 2), but the GIRLS placement (sex rule)
	// stays out of it.
	expectCodes(t, upsert(q, []string{"s1"}, []string{"Student One"},
		[]string{"Y10"}, []string{"F"}),
		"grade:s1:Y9ONLY", "budget:s1")

	if n := count(t, pool, `SELECT count(*) FROM students
		WHERE id = 's1' AND grade_id = 'Y9'`); n != 1 {
		t.Fatal("a refused upsert must leave the row unchanged")
	}

	// Accepted, the move lands.
	if err := upsert(q, []string{"s1"}, []string{"Student One"},
		[]string{"Y10"}, []string{"F"},
		"grade:s1:Y9ONLY", "budget:s1"); err != nil {
		t.Fatalf("accepted move: %v", err)
	}

	// A sex change re-judges the sex rule only: s1 (now F, in
	// GIRLS) flipping to M violates GIRLS, with no grade or
	// budget noise despite the standing sins accepted above.
	expectCodes(t, upsert(q, []string{"s1"}, []string{"Student One"},
		[]string{"Y10"}, []string{"M"}),
		"legal_sex:s1:GIRLS")
}

func TestUpsertMalformedCollection(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedStudentWrites(t, pool)

	// Malformed elements are collected with their indices,
	// and the batch is atomic.
	expectMalformed(t, upsert(q,
		[]string{"s3", "BAD ID", "s4", "s5"},
		[]string{"Fine", "Bad Id", "Bad Sex", "Bad Grade"},
		[]string{"Y9", "Y9", "Y9", "Y99"},
		[]string{"F", "F", "Q", "F"}),
		2, 3, 4)

	if n := count(t, pool, `SELECT count(*) FROM students`); n != 0 {
		t.Fatalf("a batch with malformed elements must land nothing; %d rows", n)
	}

	// Length mismatch is a hard parameter error.
	expectState(t, upsert(q, []string{"s3"}, []string{"A", "B"},
		[]string{"Y9"}, []string{"F"}), "22023")
}

// A file where every row is bad is the ordinary mistake — the wrong
// spreadsheet, or one whose columns have shifted — and it has to come
// back as a list of bad rows.
//
// It did not. The rejections were gathered with jsonb ||, which
// reserialises everything gathered so far on every append, so the cost
// was quadratic in the number of bad rows: 0.13 s at a thousand, 1.8 s
// at four thousand, 29.7 s at sixteen thousand. Thirty seconds is the
// write timeout, so past that the administrator was told "The system
// is busy or briefly unavailable. Please try again in a moment.",
// after half a minute of a core and a pool connection, and was told it
// again on every retry.
//
// The count stays true; only the described list is bounded. This
// asserts both halves, because a cap that also capped the count would
// be a lie about how much of the file is wrong.
func TestABatchOfNothingButBadRowsIsStillAListOfBadRows(t *testing.T) {
	t.Parallel()

	pool, q := fresh(t)
	seedStudentWrites(t, pool)

	// Well past max_reported_elements(), and enough that the old
	// quadratic accumulation is plainly visible against the linear one.
	const n = 4000

	ids := make([]string, n)
	names := make([]string, n)
	grades := make([]string, n)
	sexes := make([]string, n)

	for i := range n {
		// Spaces are not in the localpart grammar, so every element is
		// refused by the domain rather than by anything downstream.
		ids[i] = fmt.Sprintf("not a localpart %d", i)
		names[i] = "Name"
		grades[i] = "Y9"
		sexes[i] = "F"
	}

	start := time.Now()
	err := q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
		PIds: ids, PNames: names, PGradeIds: grades,
		PLegalSexes: sexes, PAccept: []string{},
	})
	elapsed := time.Since(start)

	pgErr := pgError(t, err, "YKD01")

	// The whole point of the exercise: the administrator learns how
	// many rows are wrong, not a number bounded by our own cap.
	if want := fmt.Sprintf("%d malformed element(s)", n); pgErr.Message != want {
		t.Errorf("message = %q, want %q", pgErr.Message, want)
	}

	var ms []malformed
	if err := json.Unmarshal([]byte(pgErr.Detail), &ms); err != nil {
		t.Fatalf("decode DETAIL: %v", err)
	}

	if len(ms) == 0 || len(ms) > 100 {
		t.Errorf("described %d elements, want between 1 and the cap of 100",
			len(ms))
	}

	// Enough of the file to say what is wrong with it.
	if ms[0].Index != 1 || !strings.Contains(ms[0].Message, "localpart") {
		t.Errorf("first rejection = %+v, want element 1 naming the domain",
			ms[0])
	}

	// A bound rather than a benchmark: the failure this rules out is
	// half a minute, and the measured time here is a fifth of a second.
	if elapsed > 10*time.Second {
		t.Errorf("took %v for %d bad rows; the write timeout is 30s",
			elapsed, n)
	}

	if n := count(t, pool, `SELECT count(*) FROM students`); n != 0 {
		t.Errorf("%d students were written by a batch that was refused", n)
	}
}

// Correcting somebody's year group must not be blocked by a budget
// decision an administrator has already taken.
//
// The budget rule counts a student's periods against their grade's
// cap. Moving between two grades that cap the same number cannot
// change that comparison — the student occupies what they occupied and
// the bound is what it was — but the rule was re-run on any grade
// change, so a student placed over the cap by an accepted exception
// had that exception resurface on an ordinary roster re-import. The
// same shape as set_enrollment_policy's budget scoping: judge the
// transition, not the statement.
func TestUpsertStudentsDoesNotJudgeABudgetTheMoveCannotMove(t *testing.T) {
	t.Parallel()

	pool, q := fresh(t)

	// Two grades, one cap between them. TWICE occupies two periods, so
	// s1 sits at 2 against a cap of 1 — a state only an accepted
	// exception can reach, which is the point.
	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods,
				min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', 1, 0, 1),
			('Y9B', 'Year 9B', 1, 0, 2),
			('Y10', 'Year 10', 5, 0, 3);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2);
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id)
			VALUES ('TWICE', 'Two periods', '', 10, FALSE, '', '', '',
				'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1');
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'Y9', 'F');
		INSERT INTO enrollments (student_id, course_id, student_droppable,
			counts_toward_budget) VALUES ('s1', 'TWICE', TRUE, TRUE)`)

	ctx := context.Background()

	// Y9 to Y9B: different grade, same cap, so nothing about the
	// budget comparison has moved.
	if err := q.UpsertStudents(ctx, db.UpsertStudentsParams{
		PIds: []string{"s1"}, PNames: []string{"Student One"},
		PGradeIds: []string{"Y9B"}, PLegalSexes: []string{"F"},
		PAccept: []string{},
	}); err != nil {
		t.Fatalf("a move between equally capped grades was refused: %v", err)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM students WHERE id = 's1' AND grade_id = 'Y9B'`); n != 1 {
		t.Fatal("the move did not land")
	}
}

// The other direction, and what keeps the scoping above honest: a move
// to a grade that caps fewer periods really can put the student over,
// so it is still judged and still refused.
func TestUpsertStudentsStillJudgesAMoveToATighterCap(t *testing.T) {
	t.Parallel()

	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports');
		INSERT INTO grades (id, name, max_budgeted_periods,
				min_distinct_categories, sort_order) VALUES
			('LOOSE', 'Loose', 5, 0, 1),
			('TIGHT', 'Tight', 1, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2);
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id)
			VALUES ('TWICE', 'Two periods', '', 10, FALSE, '', '', '',
				'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('TWICE', 'MON1'), ('TWICE', 'TUE1');
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'LOOSE', 'F');
		INSERT INTO enrollments (student_id, course_id, student_droppable,
			counts_toward_budget) VALUES ('s1', 'TWICE', TRUE, TRUE)`)

	err := q.UpsertStudents(context.Background(), db.UpsertStudentsParams{
		PIds: []string{"s1"}, PNames: []string{"Student One"},
		PGradeIds: []string{"TIGHT"}, PLegalSexes: []string{"F"},
		PAccept: []string{},
	})

	expectCodes(t, err, "budget:s1")

	if n := count(t, pool,
		`SELECT count(*) FROM students WHERE id = 's1' AND grade_id = 'LOOSE'`); n != 1 {
		t.Error("a refused move still changed the grade")
	}
}
