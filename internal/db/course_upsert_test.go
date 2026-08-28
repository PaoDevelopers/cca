package db_test

import (
	"context"
	"encoding/json/v2"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// upsert_courses: insert/update semantics, per-element malformed
// reporting (YKD01), changed-field re-judge scoping, and batch
// atomicity.

// courseSpec is one element of the JSONB the function takes. Written
// as a map rather than a struct so a test can leave a field out or put
// a wrong type in it, which is the whole point of half of these.
type courseSpec map[string]any

func upsertCourses(q *db.Queries, courses []courseSpec, accept ...string) error {
	payload, err := json.Marshal(courses)
	if err != nil {
		return err //nolint:wrapcheck // a test fixture that cannot fail in practice
	}

	//nolint:wrapcheck // the caller classifies this by SQLSTATE
	return q.UpsertCourses(context.Background(), db.UpsertCoursesParams{
		PCourses: payload,
		PAccept:  accept,
	})
}

// A well-formed element, for tests that vary one field.
func course(id string, fields courseSpec) courseSpec {
	out := courseSpec{
		"id":            id,
		"name":          id + " course",
		"description":   "",
		"category_id":   "SPORT",
		"teacher":       "",
		"teacher_email": "",
		"location":      "",
		"term":          "Season",
		"cost":          "",
		"invite_only":   false,
		"max_students":  10,
		"period_ids":    []string{},
		"legal_sexes":   []string{},
		"grade_ids":     []string{},
	}
	maps.Copy(out, fields)

	return out
}

// seedCourseUpsert: two grades, three periods, one category, and two
// students in Y9 whose enrollments the re-judging acts on.
func seedCourseUpsert(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES
			('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, max_budgeted_periods,
				min_distinct_categories, sort_order) VALUES
			('Y9', 'Year 9', 4, 0, 1),
			('Y10', 'Year 10', NULL, 0, 2);
		INSERT INTO periods (id, name, sort_order) VALUES
			('MON1', 'Monday', 1), ('TUE1', 'Tuesday', 2), ('WED1', 'Wednesday', 3);
		INSERT INTO students (id, name, grade_id, legal_sex) VALUES
			('s1', 'Student One', 'Y9', 'F'),
			('s2', 'Student Two', 'Y9', 'M')`)
}

// The ordinary path: a file of new courses lands, and re-importing it
// unchanged changes nothing and reports nothing.
func TestUpsertCoursesInsertsAndIsIdempotent(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	file := []courseSpec{
		course("SWIM", courseSpec{
			"name": "Swimming", "period_ids": []string{"MON1"},
			"max_students": 20, "teacher": "Mel",
		}),
		course("ARTS", courseSpec{
			"name": "Art club", "category_id": "ART",
			"period_ids": []string{"TUE1"}, "grade_ids": []string{"Y9"},
		}),
	}

	if err := upsertCourses(q, file); err != nil {
		t.Fatalf("first import: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses`); n != 2 {
		t.Fatalf("courses = %d, want 2", n)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_periods`); n != 2 {
		t.Fatalf("course_periods = %d, want 2", n)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_allowed_grades
		WHERE course_id = 'ARTS' AND grade_id = 'Y9'`); n != 1 {
		t.Fatalf("the grade restriction was not applied")
	}

	// Again, byte for byte. An import that had to be a fresh season
	// every time would make "the spreadsheet changed, load it again"
	// impossible.
	if err := upsertCourses(q, file); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses`); n != 2 {
		t.Fatalf("re-import changed the course count to %d", n)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_periods`); n != 2 {
		t.Fatalf("re-import changed the period count to %d", n)
	}
}

// The update half: an existing course takes the file's values, and the
// three association sets are replaced rather than merged.
func TestUpsertCoursesUpdatesAndReplacesSets(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"period_ids": []string{"MON1", "TUE1"},
			"grade_ids":  []string{"Y9", "Y10"},
		}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"name": "Swimming renamed", "max_students": 5,
			"period_ids": []string{"WED1"},
			"grade_ids":  []string{"Y10"},
		}),
	}); err != nil {
		t.Fatalf("re-import: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'SWIM' AND name = 'Swimming renamed' AND max_students = 5`); n != 1 {
		t.Fatalf("attributes were not updated")
	}

	// Replaced, not added to: the file states the arrangement it
	// wants, so what it omits is gone.
	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'SWIM'`); n != 1 {
		t.Fatalf("periods = %d, want the file's single one", n)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'SWIM' AND period_id = 'WED1'`); n != 1 {
		t.Fatalf("the period set was not replaced with the file's")
	}

	if n := count(t, pool, `SELECT count(*) FROM course_allowed_grades
		WHERE course_id = 'SWIM' AND grade_id = 'Y9'`); n != 0 {
		t.Fatalf("a grade the file omitted survived")
	}
}

// Every bad element is reported at once, with its index and id, so a
// spreadsheet is fixed in one pass rather than one row per upload.
func TestUpsertCoursesCollectsEveryMalformedElement(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	err := upsertCourses(q, []courseSpec{
		course("GOOD1", nil),
		course("lowercase", nil),                                    // 2: ill-formed id
		course("GOOD2", courseSpec{"category_id": "NOPE"}),          // 3: unknown category
		course("GOOD3", courseSpec{"max_students": "not a number"}), // 4: uncastable
		course("GOOD4", courseSpec{"legal_sexes": []string{"Q"}}),   // 5: not a legal sex
		course("GOOD5", courseSpec{"period_ids": []string{"NOPE"}}), // 6: unknown period
		course("GOOD6", courseSpec{"name": ""}),                     // 7: empty name
		course("GOOD7", nil),
	})

	ms := expectMalformed(t, err, 2, 3, 4, 5, 6, 7)

	// The id travels with the index: a spreadsheet editor needs both.
	byIndex := make(map[int]malformed, len(ms))
	for _, m := range ms {
		byIndex[m.Index] = m
	}

	if byIndex[3].ID != "GOOD2" {
		t.Errorf("element 3 reported id %q, want GOOD2", byIndex[3].ID)
	}

	if byIndex[2].ID != "lowercase" {
		t.Errorf("element 2 reported id %q", byIndex[2].ID)
	}

	// And so does the column. A domain rejection names the domain and
	// never the column — it is raised while casting, before the value
	// has reached one — so the function has to say which cast it was
	// running. Without it, five different mistakes in five different
	// columns come back as five sentences about "this".
	for index, field := range map[int]string{
		2: "id",
		3: "category",
		4: "max_students",
		5: "allowed_legal_sexes",
		6: "periods",
		7: "name",
	} {
		if byIndex[index].Field != field {
			t.Errorf("element %d reported column %q, want %q",
				index, byIndex[index].Field, field)
		}
	}
}

// Every optional display column takes '' and none of them takes
// padding, and each says which one it was. term is among the optional
// ones: a department that does not divide its season into terms leaves
// the column empty, and requiring it rejected whole spreadsheets over
// a label nothing in the software reads.
func TestUpsertCoursesNamesTheTrimmedColumn(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("BLANK", courseSpec{
			"term": "", "teacher": "", "location": "", "cost": "",
		}),
	}); err != nil {
		t.Fatalf("empty optional columns rejected: %v", err)
	}

	for i, field := range []string{
		"teacher", "teacher_email", "location", "term", "cost",
	} {
		err := upsertCourses(q, []courseSpec{
			course("PADDED", courseSpec{field: " padded"}),
		})

		ms := expectMalformed(t, err, 1)
		if ms[0].Field != field {
			t.Errorf("case %d: reported column %q, want %q",
				i, ms[0].Field, field)
		}
	}
}

// A rejected file writes nothing at all: the raise aborts the
// transaction, so the good elements go with the bad ones. Half a
// catalogue is worse than none.
func TestUpsertCoursesIsAllOrNothing(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	err := upsertCourses(q, []courseSpec{
		course("GOOD1", nil),
		course("GOOD2", courseSpec{"category_id": "NOPE"}),
	})
	expectState(t, err, "YKD01")

	if n := count(t, pool, `SELECT count(*) FROM courses`); n != 0 {
		t.Fatalf("a refused import left %d course(s) behind", n)
	}
}

// Re-scheduling a course through the import re-judges its enrollees
// for clash, exactly as editing it through the form would.
func TestUpsertCoursesRejudgesWhatItMoves(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{"period_ids": []string{"MON1"}}),
		course("CHESS", courseSpec{"period_ids": []string{"TUE1"}}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	// s1 holds both, in periods that do not clash.
	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget) VALUES
		('s1', 'SWIM', TRUE, TRUE), ('s1', 'CHESS', TRUE, TRUE)`)

	// Moving CHESS onto MON1 puts it on top of SWIM.
	err := upsertCourses(q, []courseSpec{
		course("CHESS", courseSpec{"period_ids": []string{"MON1"}}),
	})
	expectCodes(t, err, "clash:s1:CHESS:SWIM:MON1")

	// Refused means unchanged.
	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'CHESS' AND period_id = 'TUE1'`); n != 1 {
		t.Fatalf("a refused import moved the course anyway")
	}

	// Named, it proceeds.
	if err := upsertCourses(q, []courseSpec{
		course("CHESS", courseSpec{"period_ids": []string{"MON1"}}),
	}, "clash:s1:CHESS:SWIM:MON1"); err != nil {
		t.Fatalf("accepted import still refused: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'CHESS' AND period_id = 'MON1'`); n != 1 {
		t.Fatalf("the accepted import was not applied")
	}
}

// Shrinking below the enrollment is one violation for the course, not
// one per enrollee, and its code names the course alone.
func TestUpsertCoursesJudgesCapacityOncePerCourse(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{"max_students": 10}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget) VALUES
		('s1', 'SWIM', TRUE, TRUE), ('s2', 'SWIM', TRUE, TRUE)`)

	err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{"max_students": 1}),
	})
	expectCodes(t, err, "overfull:SWIM")
}

// The other half of the scoping rule: a file that changes only prose
// re-judges nobody, so a routine re-import never resurfaces a
// placement somebody already accepted.
func TestUpsertCoursesDoesNotRejudgeUnmovedRules(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	// A course only Y10 may take, holding a Y9 student: an accepted
	// violation, as an administrator's placement.
	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"period_ids": []string{"MON1"},
			"grade_ids":  []string{"Y10"},
		}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'SWIM', FALSE, FALSE)`)

	// Same sets, different prose. Nothing that any rule reads has
	// moved, so nothing is re-judged and the standing violation is
	// not raised again.
	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"name": "Swimming, renamed", "teacher": "Mel", "cost": "200 rmb",
			"period_ids": []string{"MON1"},
			"grade_ids":  []string{"Y10"},
		}),
	}); err != nil {
		t.Fatalf("a prose-only re-import was refused: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'SWIM' AND teacher = 'Mel'`); n != 1 {
		t.Fatalf("the prose edit was not applied")
	}
}

// A set given in a different order, or with a repeat, is the same set:
// comparison must see a set rather than a spelling, or a spreadsheet
// re-sorted by its author would re-judge everybody.
func TestUpsertCoursesComparesSetsNotSpellings(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"period_ids": []string{"MON1", "TUE1"},
			"grade_ids":  []string{"Y9", "Y10"},
		}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'SWIM', TRUE, TRUE)`)

	// Y9's cap is 4 and this occupies 2, so a spurious budget
	// re-judge would still pass; what would fail is the grade rule if
	// the reordered list read as a change. Reordered and duplicated:
	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{
			"period_ids": []string{"TUE1", "MON1", "MON1"},
			"grade_ids":  []string{"Y10", "Y9"},
		}),
	}); err != nil {
		t.Fatalf("a re-ordered set was treated as a change: %v", err)
	}

	// And the duplicate did not become a second row.
	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'SWIM'`); n != 2 {
		t.Fatalf("periods = %d, want 2", n)
	}
}

// The batch is not an upsert on the ids alone: an element naming a
// course that does not exist creates it, which is what makes the
// import the same operation as the form's create.
func TestUpsertCoursesMixesInsertAndUpdate(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{"max_students": 10}),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", courseSpec{"max_students": 12}),
		course("CHESS", nil),
	}); err != nil {
		t.Fatalf("mixed import: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'SWIM' AND max_students = 12`); n != 1 {
		t.Fatalf("the existing course was not updated")
	}

	if n := count(t, pool, `SELECT count(*) FROM courses WHERE id = 'CHESS'`); n != 1 {
		t.Fatalf("the new course was not created")
	}
}

// An empty file is a no-op rather than an error: an administrator who
// uploads a header-only spreadsheet has said nothing, not asked for
// everything to be deleted.
func TestUpsertCoursesAcceptsAnEmptyBatch(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{}); err != nil {
		t.Fatalf("empty import: %v", err)
	}
}

// Absence from a file is not evidence a course was cancelled, so an
// import never deletes. Removing a course stays a deliberate act.
func TestUpsertCoursesNeverDeletes(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("SWIM", nil), course("CHESS", nil),
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	if err := upsertCourses(q, []courseSpec{course("SWIM", nil)}); err != nil {
		t.Fatalf("second import: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses WHERE id = 'CHESS'`); n != 1 {
		t.Fatalf("a course absent from the file was deleted")
	}
}

// The spreadsheet's own text reaches the casts, so the spellings a
// spreadsheet actually produces have to work — and an unfilled
// invite-only column must mean "not invite-only" rather than a cast
// failure, because that is what an empty cell means to its author.
func TestUpsertCoursesReadsSpreadsheetText(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	for _, tt := range []struct {
		cell string
		want bool
	}{
		{"", false},
		{"FALSE", false},
		{"no", false},
		{"n", false},
		{"0", false},
		{"TRUE", true},
		{"true", true},
		{"yes", true},
		{"y", true},
		{"1", true},
	} {
		if err := upsertCourses(q, []courseSpec{
			course("SWIM", courseSpec{
				"invite_only":  tt.cell,
				"max_students": "10",
			}),
		}); err != nil {
			t.Fatalf("invite_only %q: %v", tt.cell, err)
		}

		got := count(t, pool, `SELECT count(*) FROM courses
			WHERE id = 'SWIM' AND invite_only`) == 1
		if got != tt.want {
			t.Errorf("invite_only %q read as %v, want %v", tt.cell, got, tt.want)
		}
	}

	// A capacity given as text casts; one that is not a number is a
	// malformed element rather than a failure of the whole call.
	if err := upsertCourses(q, []courseSpec{
		course("CHESS", courseSpec{"max_students": "25"}),
	}); err != nil {
		t.Fatalf("numeric text capacity: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'CHESS' AND max_students = 25`); n != 1 {
		t.Fatalf("the capacity was not read from its text")
	}

	expectMalformed(t,
		upsertCourses(q, []courseSpec{
			course("GOOD", nil),
			course("BAD1", courseSpec{"invite_only": "perhaps"}),
			course("BAD2", courseSpec{"max_students": "-1"}),
		}),
		2, 3)
}

// Two courses trading timetable slots is an ordinary re-import, and it
// must not be refused.
//
// It was. Judging happened inside the apply loop, so the first element
// was judged while every later element still held its old periods —
// AAA had already moved into Monday while BBB was still there. The
// administrator was shown "clashes with BBB in MON1" for a clash that
// does not exist before the import and does not exist after it, and
// the only way past was to accept a violation that was never true,
// which is the one thing the accept protocol exists to make
// unnecessary. It is also 0012's charter read properly: the stored
// state is the hypothesis, and the hypothesis is not finished until
// the last element has landed.
func TestUpsertCoursesDoesNotJudgeAHalfAppliedCatalogue(t *testing.T) {
	t.Parallel()

	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
			invite_only, teacher, teacher_email, location, term, cost,
			category_id) VALUES
			('AAA', 'A', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('BBB', 'B', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('AAA', 'MON1'), ('BBB', 'TUE1');
		INSERT INTO enrollments (student_id, course_id, student_droppable,
			counts_toward_budget) VALUES
			('s1', 'AAA', TRUE, TRUE), ('s1', 'BBB', TRUE, TRUE)`)

	// They swap. s1 holds both, and holds one period in each before
	// and after, so nothing about their timetable actually changes.
	err := upsertCourses(q, []courseSpec{
		course("AAA", courseSpec{"period_ids": []string{"TUE1"}}),
		course("BBB", courseSpec{"period_ids": []string{"MON1"}}),
	})
	if err != nil {
		t.Fatalf("a slot swap was refused: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'AAA' AND period_id = 'TUE1'`); n != 1 {
		t.Error("the swap did not land")
	}
}

// The other direction, and the one that makes the fix above safe
// rather than merely quiet: an import that genuinely does collide two
// courses is still refused, and names the pair.
func TestUpsertCoursesStillCatchesAClashItCreates(t *testing.T) {
	t.Parallel()

	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
			invite_only, teacher, teacher_email, location, term, cost,
			category_id) VALUES
			('AAA', 'A', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT'),
			('BBB', 'B', '', 10, FALSE, '', '', '', 'Season', '', 'SPORT');
		INSERT INTO course_periods (course_id, period_id) VALUES
			('AAA', 'MON1'), ('BBB', 'TUE1');
		INSERT INTO enrollments (student_id, course_id, student_droppable,
			counts_toward_budget) VALUES
			('s1', 'AAA', TRUE, TRUE), ('s1', 'BBB', TRUE, TRUE)`)

	// Both into Monday: s1 now cannot attend both.
	err := upsertCourses(q, []courseSpec{
		course("AAA", courseSpec{"period_ids": []string{"MON1"}}),
		course("BBB", courseSpec{"period_ids": []string{"MON1"}}),
	})

	expectCodes(t, err, "clash:s1:AAA:BBB:MON1")

	if n := count(t, pool, `SELECT count(*) FROM course_periods
		WHERE course_id = 'BBB' AND period_id = 'MON1'`); n != 0 {
		t.Error("a refused import still moved a course")
	}
}

// A blank capacity cell and the word "unlimited" both mean no cap,
// which is NULL. A department that has no limit in mind writes one or
// the other, and neither is a number: both used to be a cast failure
// on every row that had no cap to state, which is not a mistake in the
// file. NULL is also a different setting from 0, which is a real cap
// that admits nobody.
func TestUpsertCoursesReadsAnAbsentCapacity(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("BLANK", courseSpec{"max_students": ""}),
		course("WORD", courseSpec{"max_students": "unlimited"}),
		// A spreadsheet's own capitalisation and padding.
		course("SHOUTED", courseSpec{"max_students": " Unlimited "}),
		course("ZERO", courseSpec{"max_students": 0}),
		course("TEN", courseSpec{"max_students": "10"}),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	for _, id := range []string{"BLANK", "WORD", "SHOUTED"} {
		if n := count(t, pool, `SELECT count(*) FROM courses
			WHERE id = '`+id+`' AND max_students IS NULL`); n != 1 {
			t.Errorf("%s did not come out uncapped", id)
		}
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'ZERO' AND max_students = 0`); n != 1 {
		t.Error("0 was read as no cap; it is a cap that admits nobody")
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id = 'TEN' AND max_students = 10`); n != 1 {
		t.Error("a plain number stopped being a plain number")
	}

	// Not every non-number, though: only the two that mean absence.
	ms := expectMalformed(t,
		upsertCourses(q, []courseSpec{
			course("LOTS", courseSpec{"max_students": "lots"}),
		}), 1)
	if ms[0].Field != "max_students" {
		t.Errorf("reported column %q, want max_students", ms[0].Field)
	}
}

// An uncapped course never reports being full, and the capacity rule
// gets there by reading the absence rather than testing for it:
// current_students >= NULL is NULL, so the rule yields no row.
func TestUncappedCourseIsNeverFull(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	if err := upsertCourses(q, []courseSpec{
		course("OPEN", courseSpec{
			"max_students": "unlimited",
			"period_ids":   []string{"WED1"},
		}),
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Both seeded students in, then a capacity verdict asked for a
	// third: a capped course of 0 would refuse, and this must not.
	exec(t, pool, `INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s1', 'OPEN', TRUE, TRUE), ('s2', 'OPEN', TRUE, TRUE)`)

	if c := violationCodes(t, q, "capacity",
		violations("s1", "OPEN", false)); len(c) != 0 {
		t.Errorf("an uncapped course reported itself full: %v", c)
	}

	// And lowering it to a real cap it is already over does report,
	// which is what proves the silence above was the NULL and not the
	// rule being switched off.
	err := upsertCourses(q, []courseSpec{
		course("OPEN", courseSpec{
			"max_students": 1,
			"period_ids":   []string{"WED1"},
		}),
	})
	if err == nil {
		t.Fatal("dropping the cap below the roll was accepted silently")
	}

	expectCodes(t, err, "overfull:OPEN")
}

// A file that states one id twice is refused, loudly, naming every row
// that collides.
//
// Applying them in order would leave the last one standing and report
// success: the rows go in, one course comes out, and nothing says
// which were discarded. Nobody writes a course twice in order to
// overwrite the first with the second.
func TestUpsertCoursesRefusesARepeatedID(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	err := upsertCourses(q, []courseSpec{
		course("GOOD1", nil),
		course("TWICE", courseSpec{"name": "First"}),
		course("GOOD2", nil),
		course("TWICE", courseSpec{"name": "Second"}),
		course("THRICE", nil),
		course("THRICE", nil),
		course("THRICE", nil),
	})

	// Every colliding row, not all but the first: which to keep is
	// the administrator's decision and they cannot make it from half
	// the collision.
	ms := expectMalformed(t, err, 2, 4, 5, 6, 7)

	for _, m := range ms {
		if m.SQLState != "YKD02" {
			t.Errorf("element %d sqlstate = %q, want YKD02", m.Index, m.SQLState)
		}

		if m.Field != "id" {
			t.Errorf("element %d field = %q, want id", m.Index, m.Field)
		}
	}

	// And nothing landed, including the rows that were fine.
	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id IN ('GOOD1', 'GOOD2', 'TWICE', 'THRICE')`); n != 0 {
		t.Fatalf("a refused import must leave nothing behind; %d rows", n)
	}

	// The same list with the repeat resolved goes in.
	if err := upsertCourses(q, []courseSpec{
		course("GOOD1", nil),
		course("TWICE", courseSpec{"name": "First"}),
		course("GOOD2", nil),
		course("THRICE", nil),
	}); err != nil {
		t.Fatalf("a file with no repeat was refused: %v", err)
	}
}

// Re-importing the same file is still the supported way to work: the
// repeat that matters is within one batch, not between two calls.
func TestUpsertCoursesStillAllowsAReImport(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedCourseUpsert(t, pool)

	file := []courseSpec{course("YOGA", nil), course("CHESS", nil)}

	for attempt := range 2 {
		if err := upsertCourses(q, file); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}

	if n := count(t, pool, `SELECT count(*) FROM courses
		WHERE id IN ('YOGA', 'CHESS')`); n != 2 {
		t.Fatalf("a re-import must change nothing; %d rows", n)
	}
}
