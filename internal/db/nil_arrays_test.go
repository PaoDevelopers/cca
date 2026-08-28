package db_test

import (
	"context"
	"testing"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Go nil slices arrive in PostgreSQL as NULL arrays,
// and x = ANY (NULL) is NULL, not false —
// so an unguarded rule predicate would silently void itself.
// The functions COALESCE every array parameter to '{}';
// these tests pin that a nil disregard list still finds clashes
// and, far more importantly, that a nil accept list accepts
// NOTHING rather than everything.
func TestNilArraysReadAsEmpty(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedViolations(t, pool)

	ctx := context.Background()

	// nil disregard: the clash must still be found.
	rows, err := q.EnrollmentViolations(ctx, db.EnrollmentViolationsParams{
		PStudentID: "s1", PCourseID: "ARTMON",
		PCountsTowardBudget: false, PDisregardCourseIds: nil,
	})
	if err != nil {
		t.Fatalf("EnrollmentViolations: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("nil disregard voided the clash rule: %v", rows)
	}

	// nil accept on a violating placement: must refuse, not
	// silently accept everything.
	err = q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID:   "TINY", // s2 holds its only seat
		PStudentIds: []string{"s3"},
		PAccept:     nil,
	})
	expectCodes(t, err, "capacity:s3:TINY")

	// nil accept on a violating capacity shrink: likewise. A
	// different rule from the one above, and so a different code:
	// "capacity" is whether this student fits, "overfull" is whether
	// the course now holds more than it says it can.
	err = q.UpdateCourse(ctx, db.UpdateCourseParams{
		PCourseID: "TINY", PName: "One seat", PCategoryID: "ART",
		PTerm: "Season", PMaxStudents: capacity(0),
		PPeriodIds: []string{"WED1"}, PAccept: nil,
	})
	expectCodes(t, err, "overfull:TINY")
}
