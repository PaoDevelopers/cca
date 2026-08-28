package db_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Course writes: creation and editing as whole-course operations,
// the rules an edit re-judges and the ones it must leave alone,
// and deletion.

// seedCourses: MAIN meets MON1 and holds s1 (F, Y9, charging),
// s2 (M, Y9, not charging) and s3 (M, Y10, not charging).
// OTHER meets TUE1 and holds s2, so growing MAIN into TUE1 clashes.
// GIRLS is restricted to F and holds s2 on an accepted violation,
// so any edit that re-judges too widely will resurface it.
// Y9 caps the budget at 3 periods, which only s1 charges against.
func seedCourses(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES ('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', 3, 0, 1),
			('Y10', 'Year 10', NULL, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2),
			('WED1', 'Wednesday', 3), ('THU1', 'Thursday', 4);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'F Nine', 'Y9', 'F'),
			('s2', 'M Nine', 'Y9', 'M'),
			('s3', 'M Ten', 'Y10', 'M');
		INSERT INTO courses (id, name, description, max_students, invite_only,
				teacher, teacher_email, location, term, cost, category_id) VALUES
			('MAIN', 'Main course', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('GIRLS', 'Girls only', '', 10, FALSE, '', '', '', 'Season', '', 'ART'),
			('OTHER', 'Tuesday course', '', 10, FALSE, '', '', '', 'Season', '', 'ART');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('MAIN', 'MON1'), ('OTHER', 'TUE1');
		INSERT INTO course_allowed_legal_sexes (course_id, legal_sex) VALUES ('GIRLS', 'F');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget) VALUES
			('s1', 'MAIN', TRUE, TRUE),
			('s2', 'MAIN', TRUE, FALSE),
			('s3', 'MAIN', FALSE, FALSE),
			('s2', 'OTHER', TRUE, FALSE),
			('s2', 'GIRLS', FALSE, FALSE)`)
}

// mainCourse is MAIN exactly as seeded. Editing is declarative, so
// every call restates the whole course; a test changes one field of
// this and leaves the rest alone, which is also how a form behaves.
func mainCourse() db.UpdateCourseParams {
	return db.UpdateCourseParams{
		PCourseID:    "MAIN",
		PName:        "Main course",
		PCategoryID:  "SPORT",
		PTerm:        "Season",
		PMaxStudents: capacity(10),
		PPeriodIds:   []string{"MON1"},
	}
}

func storedPeriods(t *testing.T, pool *pgxpool.Pool, course string) []string {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		`SELECT period_id::TEXT FROM course_periods
		WHERE course_id = $1 ORDER BY period_id`, course)
	if err != nil {
		t.Fatalf("periods: %v", err)
	}
	defer rows.Close()

	var out []string

	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}

		out = append(out, p)
	}

	return out
}

func TestCreateCourse(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	// One call carries the attributes, the timetable and both
	// restrictions: a course is never briefly half-created.
	if err := q.CreateCourse(ctx, db.CreateCourseParams{
		PCourseID:    "NEW",
		PName:        "New course",
		PDescription: "Two lines\nof prose",
		PCategoryID:  "ART",
		PTeacher:     "Ms Smith",
		PTerm:        "Season",
		PMaxStudents: capacity(5),
		PInviteOnly:  true,
		PPeriodIds:   []string{"WED1", "THU1", "WED1"},
		PLegalSexes:  []db.LegalSex{db.LegalSexF},
		PGradeIds:    []string{"Y9"},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	c := courseByID(t, q, "NEW")
	if c.Name != "New course" || c.Teacher != "Ms Smith" ||
		c.MaxStudents.Int64 != 5 || !c.InviteOnly || c.CategoryID != "ART" {
		t.Fatalf("attributes must be stored as given: %+v", c)
	}

	if got := storedPeriods(t, pool, "NEW"); !slices.Equal(got, []string{"THU1", "WED1"}) {
		t.Fatalf("a repeated period is a set member, not a duplicate: %v", got)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_allowed_grades
		WHERE course_id = 'NEW'`); n != 1 {
		t.Fatalf("restrictions must be stored with the course; %d rows", n)
	}

	// No cap is a setting the form can ask for, and it is not 0.
	if err := q.CreateCourse(ctx, db.CreateCourseParams{
		PCourseID:    "OPEN",
		PName:        "Takes everyone",
		PCategoryID:  "ART",
		PTerm:        "Season",
		PMaxStudents: uncapped,
	}); err != nil {
		t.Fatalf("create uncapped: %v", err)
	}

	if open := courseByID(t, q, "OPEN"); open.MaxStudents.Valid {
		t.Fatalf("no cap was stored as %d", open.MaxStudents.Int64)
	}

	// A new course has no enrollees, so nothing can be violated and
	// there is no accept parameter to offer.
	if c.CurrentStudents != 0 {
		t.Fatalf("a new course starts empty: %+v", c)
	}

	// Failures are whole-course: no orphaned periods survive.
	expectState(t, q.CreateCourse(ctx, db.CreateCourseParams{
		PCourseID: "BAD", PName: "Bad", PCategoryID: "NOPE",
		PTerm: "Season", PPeriodIds: []string{"WED1"},
	}), "23503")

	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'BAD'`); n != 0 {
		t.Fatalf("a failed creation must leave nothing behind; %d rows", n)
	}
}

// The property the composite exists for: an edit that touches
// several things is one transaction, so a refusal on any of them
// leaves all of them unapplied.
func TestUpdateCourseIsOneTransaction(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	p := mainCourse()
	p.PName = "Renamed"
	p.PPeriodIds = []string{"MON1", "TUE1"}

	// The rename is fine; the timetable clashes for s2.
	expectCodes(t, q.UpdateCourse(ctx, p), "clash:s2:MAIN:OTHER:TUE1")

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'MAIN' AND name = 'Main course'`); n != 1 {
		t.Fatal("a refused edit must not apply the fields that were fine")
	}

	if got := storedPeriods(t, pool, "MAIN"); !slices.Equal(got, []string{"MON1"}) {
		t.Fatalf("a refused edit must leave the timetable alone: %v", got)
	}

	// Accepted, the whole edit lands together.
	p.PAccept = []string{"clash:s2:MAIN:OTHER:TUE1"}
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("accepted edit: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'MAIN' AND name = 'Renamed'`); n != 1 {
		t.Fatal("an accepted edit must apply every field")
	}

	if got := storedPeriods(t, pool, "MAIN"); !slices.Equal(got, []string{"MON1", "TUE1"}) {
		t.Fatalf("stored = %v", got)
	}
}

func TestUpdateCourseScalarsJudgeNobody(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	// Put MAIN in a state where two students violate a stored
	// restriction, by accepting it once.
	p := mainCourse()
	p.PLegalSexes = []db.LegalSex{db.LegalSexF}
	p.PAccept = []string{"legal_sex:s2:MAIN", "legal_sex:s3:MAIN"}

	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("accepted restriction: %v", err)
	}

	// Now edit only rule-free fields. Nothing moved that any rule
	// reads, so the accepted violations must stay settled.
	p = mainCourse()
	p.PLegalSexes = []db.LegalSex{db.LegalSexF}
	p.PName = "Renamed"
	p.PTeacher = "Ms Smith"
	p.PLocation = "Gym"
	p.PInviteOnly = true

	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("a rule-free edit must need no accepts: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'MAIN' AND name = 'Renamed' AND teacher = 'Ms Smith'
			AND location = 'Gym' AND invite_only`); n != 1 {
		t.Fatal("rule-free fields must apply")
	}
}

func TestUpdateCourseRestrictions(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	// Restricting legal sex to F re-judges the two enrolled boys.
	p := mainCourse()
	p.PLegalSexes = []db.LegalSex{db.LegalSexF}
	expectCodes(t, q.UpdateCourse(ctx, p),
		"legal_sex:s2:MAIN", "legal_sex:s3:MAIN")

	if n := count(t, pool, `SELECT count(*) FROM course_allowed_legal_sexes
		WHERE course_id = 'MAIN'`); n != 0 {
		t.Fatalf("a refused restriction must not be stored; %d rows", n)
	}

	p.PAccept = []string{"legal_sex:s2:MAIN", "legal_sex:s3:MAIN"}
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("accepted restriction: %v", err)
	}

	// Widening back to unrestricted violates nothing: the protocol
	// is applied uniformly and comes back empty on its own.
	if err := q.UpdateCourse(ctx, mainCourse()); err != nil {
		t.Fatalf("clearing a restriction must need no accepts: %v", err)
	}

	// Each restriction re-judges only its own rule. With the F
	// restriction stored again, a grade restriction must not
	// resurface the accepted legal_sex violations.
	p = mainCourse()
	p.PLegalSexes = []db.LegalSex{db.LegalSexF}
	p.PAccept = []string{"legal_sex:s2:MAIN", "legal_sex:s3:MAIN"}

	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("restore restriction: %v", err)
	}

	p.PGradeIds = []string{"Y9"}
	p.PAccept = nil
	expectCodes(t, q.UpdateCourse(ctx, p), "grade:s3:MAIN")
}

func TestUpdateCoursePeriods(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	// A clean reschedule applies.
	p := mainCourse()
	p.PPeriodIds = []string{"MON1", "WED1"}

	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("clean reschedule: %v", err)
	}

	if got := storedPeriods(t, pool, "MAIN"); !slices.Equal(got, []string{"MON1", "WED1"}) {
		t.Fatalf("stored = %v", got)
	}

	// Growing to four periods pushes s1 (charging, cap 3) over
	// budget; s2 and s3 do not charge and are exempt.
	p.PPeriodIds = []string{"MON1", "WED1", "THU1", "TUE1"}
	expectCodes(t, q.UpdateCourse(ctx, p),
		"clash:s2:MAIN:OTHER:TUE1", "budget:s1")

	// Unknown periods fail on the foreign key, atomically.
	p.PPeriodIds = []string{"NOPE"}
	expectState(t, q.UpdateCourse(ctx, p), "23503")

	if got := storedPeriods(t, pool, "MAIN"); !slices.Equal(got, []string{"MON1", "WED1"}) {
		t.Fatalf("a failed reschedule must leave the stored set untouched: %v", got)
	}

	// Unscheduling entirely charges nothing and clashes with
	// nothing, so it needs no accepts.
	p.PPeriodIds = nil
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("unscheduling must need no accepts: %v", err)
	}

	if got := storedPeriods(t, pool, "MAIN"); got != nil {
		t.Fatalf("an empty timetable must clear the periods: %v", got)
	}
}

func TestUpdateCourseCapacity(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	// Shrinking below the enrollment is one course-level violation,
	// not one per enrollee: its code names the course alone.
	p := mainCourse()
	p.PMaxStudents = capacity(2)
	expectCodes(t, q.UpdateCourse(ctx, p), "overfull:MAIN")

	if n := count(t, pool, `SELECT max_students FROM courses WHERE id = 'MAIN'`); n != 10 {
		t.Fatalf("a refused cap change must not be stored; cap %d", n)
	}

	p.PAccept = []string{"overfull:MAIN"}
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("accepted shrink: %v", err)
	}

	if c := courseByID(t, q, "MAIN"); c.MaxStudents.Int64 != 2 || c.CurrentStudents != 3 {
		t.Fatalf("shrink must apply and stay visible: %+v", c)
	}

	// Re-saving the course without changing the cap must not
	// re-raise the capacity violation: the cap did not move, so it
	// is not this edit's business.
	p.PAccept = nil
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("an unchanged cap must not be re-judged: %v", err)
	}

	// Shrinking to exactly the enrollment needs no accepts.
	p.PMaxStudents = capacity(3)
	if err := q.UpdateCourse(ctx, p); err != nil {
		t.Fatalf("a cap meeting the enrollment exactly must pass: %v", err)
	}
}

func TestCourseWriteExistence(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)

	ctx := context.Background()

	p := mainCourse()
	p.PCourseID = "NOPE"
	expectState(t, q.UpdateCourse(ctx, p), "P0002")
	expectState(t, q.DeleteCourse(ctx, "NOPE"), "P0002")
}

func TestDeleteCourse(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourses(t, pool)
	exec(t, pool, `INSERT INTO course_allowed_grades (course_id, grade_id)
		VALUES ('MAIN', 'Y9')`)

	ctx := context.Background()

	// Deletion removes enrollments, periods, and restrictions with
	// the course; students survive.
	if err := q.DeleteCourse(ctx, "MAIN"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	for what, sql := range map[string]string{
		"the course":       `SELECT count(*) FROM courses WHERE id = 'MAIN'`,
		"its enrollments":  `SELECT count(*) FROM enrollments WHERE course_id = 'MAIN'`,
		"its periods":      `SELECT count(*) FROM course_periods WHERE course_id = 'MAIN'`,
		"its restrictions": `SELECT count(*) FROM course_allowed_grades WHERE course_id = 'MAIN'`,
	} {
		if n := count(t, pool, sql); n != 0 {
			t.Fatalf("%s must be gone; %d left", what, n)
		}
	}

	if n := count(t, pool, `SELECT count(*) FROM students`); n != 3 {
		t.Fatalf("students must survive course deletion; %d left", n)
	}
}
