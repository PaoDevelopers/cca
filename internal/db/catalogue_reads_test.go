package db_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The two whole-catalogue reads the application layer adds on top of
// the per-enrollment ones: student_course_violations, which judges
// every candidate course for one student in a single call, and
// v_export_enrollments, the flat PowerSchool hand-off.

// courseViolations runs the batched read and returns, per course, its
// sorted violation codes. Courses with no violations appear with an
// empty slice; courses absent from the result are absent from the map.
func courseViolations(t *testing.T, q *db.Queries, student string, charging bool) map[string][]string {
	t.Helper()

	rows, err := q.StudentCourseViolations(context.Background(),
		db.StudentCourseViolationsParams{
			PStudentID:          student,
			PCountsTowardBudget: charging,
		})
	if err != nil {
		t.Fatalf("StudentCourseViolations: %v", err)
	}

	byCourse := make(map[string][]string)

	for _, r := range rows {
		if !r.CourseID.Valid {
			t.Fatalf("course_id must never be NULL: %+v", r)
		}

		byCourse[r.CourseID.String] = append(byCourse[r.CourseID.String], r.Code.String)
	}

	for _, codes := range byCourse {
		slices.Sort(codes)
	}

	return byCourse
}

// The batched read must agree with the per-course one, course for
// course. That equivalence is the whole reason it may exist: it is an
// optimisation of the round trips, not a second statement of the
// rules.
func TestStudentCourseViolationsAgreesWithPerCourse(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	for _, student := range []string{"s1", "s2", "s3"} {
		for _, charging := range []bool{true, false} {
			batched := courseViolations(t, q, student, charging)

			for _, course := range []string{"TWICE", "ARTMON", "TINY", "GIRLS", "Y9GRADE"} {
				held := count(t, pool,
					`SELECT count(*) FROM enrollments WHERE student_id = '`+student+
						`' AND course_id = '`+course+`'`)

				// A held course is not a candidate, so the batched
				// read omits it however it would have judged.
				if held > 0 {
					if _, ok := batched[course]; ok {
						t.Errorf("%s/%s: held course present in batched read", student, course)
					}

					continue
				}

				want := violationCodes(t, q, "", violations(student, course, charging))
				got := batched[course]

				if len(want) == 0 && len(got) == 0 {
					continue
				}

				if !slices.Equal(got, want) {
					t.Errorf("%s/%s charging=%v: batched %v, per-course %v",
						student, course, charging, got, want)
				}
			}
		}
	}
}

// Every candidate course appears, including the ones that violate
// nothing: the catalogue needs a verdict for each, and "absent" would
// be indistinguishable from "not judged".
func TestStudentCourseViolationsCoversEveryCandidate(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// s3 (Y10, M, uncapped) holds nothing, so every course is a
	// candidate. Each restricted one reports its own rule.
	got := courseViolations(t, q, "s3", true)

	if codes := got["Y9GRADE"]; !slices.Equal(codes, []string{"grade:s3:Y9GRADE"}) {
		t.Errorf("Y9GRADE for s3: %v", codes)
	}

	if codes := got["GIRLS"]; !slices.Equal(codes, []string{"legal_sex:s3:GIRLS"}) {
		t.Errorf("GIRLS for s3: %v", codes)
	}

	// Unrestricted, roomy, and clashing with nothing s3 holds.
	for _, course := range []string{"TWICE", "ARTMON"} {
		if codes, ok := got[course]; ok {
			t.Errorf("%s for s3 should be clean, got %v", course, codes)
		}
	}

	// TINY is full, and fullness is a violation of a candidate course
	// rather than a reason to hide it.
	if codes := got["TINY"]; !slices.Equal(codes, []string{"capacity:s3:TINY"}) {
		t.Errorf("TINY for s3: %v", codes)
	}
}

// A course the student already holds is not judged: its seat is
// already theirs, so reporting its capacity against them would be a
// false negative in the one place students read for eligibility.
func TestStudentCourseViolationsExcludesHeldCourses(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// s2 holds TINY, the full course.
	if codes, ok := courseViolations(t, q, "s2", true)["TINY"]; ok {
		t.Errorf("held TINY judged for its own holder: %v", codes)
	}

	// s1 holds TWICE, so ARTMON's clash against it is still reported
	// (ARTMON is a candidate) but TWICE itself is not.
	got := courseViolations(t, q, "s1", true)

	if _, ok := got["TWICE"]; ok {
		t.Errorf("held TWICE judged for its own holder")
	}

	if codes := got["ARTMON"]; !slices.Equal(codes, []string{"clash:s1:ARTMON:TWICE:MON1"}) {
		t.Errorf("ARTMON for s1: %v", codes)
	}
}

// The export is one row per occupancy cell, and an unscheduled course
// still exports its enrollees with a NULL period rather than dropping
// them.
func TestExportEnrollments(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	// An unscheduled course with an enrollee.
	exec(t, pool, `INSERT INTO courses (id, name, description, max_students,
			invite_only, teacher, teacher_email, location, term, cost, category_id)
		VALUES ('PENDING', 'Not yet scheduled', '', 10, FALSE, '', '', '', 'Year', '', 'ART');
		INSERT INTO enrollments (student_id, course_id, student_droppable, counts_toward_budget)
		VALUES ('s3', 'PENDING', FALSE, TRUE)`)

	rows, err := q.GetEnrollmentsExport(context.Background())
	if err != nil {
		t.Fatalf("GetEnrollmentsExport: %v", err)
	}

	type cell struct {
		student string
		course  string
		period  string
	}

	got := make([]cell, 0, len(rows))

	for _, r := range rows {
		got = append(got, cell{r.StudentID, r.CourseID, r.PeriodID})
	}

	// s1 in TWICE occupies two cells, s2 in TINY one, s3 in PENDING
	// one with no period.
	want := []cell{
		{"s1", "TWICE", "MON1"},
		{"s1", "TWICE", "TUE1"},
		{"s2", "TINY", "WED1"},
		{"s3", "PENDING", ""},
	}

	if !slices.Equal(got, want) {
		t.Fatalf("export cells = %v, want %v", got, want)
	}

	// The denormalised columns carry the names the spreadsheet needs,
	// and the policy bits survive.
	for _, r := range rows {
		if r.StudentName == "" || r.CourseName == "" {
			t.Errorf("export row missing display names: %+v", r)
		}

		if r.StudentID == "s3" && !r.CountsTowardBudget {
			t.Errorf("s3/PENDING should count toward budget: %+v", r)
		}

		if r.StudentID == "s3" && r.StudentDroppable {
			t.Errorf("s3/PENDING should not be student-droppable: %+v", r)
		}
	}
}

// The boundary read behind the realtime window timer: the earliest
// bound still ahead, over every grade, or nothing when none is.
func TestNextWindowBoundary(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades (id, name, min_distinct_categories, sort_order)
		VALUES ('Y9', 'Year 9', 0, 1), ('Y10', 'Year 10', 0, 2)`)

	// No windows set at all.
	b, err := q.NextWindowBoundary(context.Background())
	if err != nil {
		t.Fatalf("NextWindowBoundary: %v", err)
	}

	if b.Valid {
		t.Fatalf("no windows set, got boundary %v", b.Time)
	}

	// Past bounds are behind us and must not arm anything.
	exec(t, pool, `UPDATE grades
		SET opens_at = now() - interval '2 days', closes_at = now() - interval '1 day'
		WHERE id = 'Y9'`)

	b, err = q.NextWindowBoundary(context.Background())
	if err != nil {
		t.Fatalf("NextWindowBoundary: %v", err)
	}

	if b.Valid {
		t.Fatalf("only past bounds, got boundary %v", b.Time)
	}

	// The earliest future bound wins, across grades and across the
	// two columns alike.
	exec(t, pool, `UPDATE grades
		SET opens_at = now() + interval '10 days', closes_at = now() + interval '20 days'
		WHERE id = 'Y9'`)
	exec(t, pool, `UPDATE grades
		SET opens_at = now() - interval '1 day', closes_at = now() + interval '3 days'
		WHERE id = 'Y10'`)

	b, err = q.NextWindowBoundary(context.Background())
	if err != nil {
		t.Fatalf("NextWindowBoundary: %v", err)
	}

	if !b.Valid {
		t.Fatal("future bounds exist, got none")
	}

	// Y10's closes_at, ~3 days out, is the earliest.
	if d := time.Until(b.Time); d < 2*24*time.Hour || d > 4*24*time.Hour {
		t.Fatalf("boundary %v is not Y10's closes_at (~3 days out)", b.Time)
	}
}
