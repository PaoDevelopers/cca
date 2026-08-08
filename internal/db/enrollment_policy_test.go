package db_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PaoDevelopers/cca/internal/db"
)

// set_enrollment_policy: changing the two policy bits of enrollments
// that already exist, without giving up the seats.

func setPolicy(q *db.Queries, course string, students []string, droppable, budgeted bool, accept ...string) error {
	if err := q.SetEnrollmentPolicy(context.Background(), db.SetEnrollmentPolicyParams{
		PCourseID:           course,
		PStudentIds:         students,
		PStudentDroppable:   droppable,
		PCountsTowardBudget: budgeted,
		PAccept:             accept,
	}); err != nil {
		return fmt.Errorf("set_enrollment_policy: %w", err)
	}

	return nil
}

// The ordinary case: an invitation becomes a placement the student may
// not decline, and the seat never moves.
func TestSetEnrollmentPolicyChangesBits(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: false, PAccept: nil,
	}); err != nil {
		t.Fatalf("place: %v", err)
	}

	if err := setPolicy(q, "SWIM", []string{"s1"}, false, false); err != nil {
		t.Fatalf("set_enrollment_policy: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1'
			AND NOT student_droppable AND NOT counts_toward_budget`); n != 1 {
		t.Fatalf("policy not applied, matching rows = %d", n)
	}

	// The seat was never released: still exactly one enrollee.
	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'SWIM'`); n != 1 {
		t.Fatalf("enrollee count = %d, want 1", n)
	}

	// And the student can no longer drop it, which is what the bit
	// means.
	if err := selfDrop(q, "s1", "SWIM"); err == nil {
		t.Fatal("a non-droppable enrollment was dropped by its student")
	}
}

// Turning the budget bit on is the one direction that can break
// something, so it is the one that reports a violation.
func TestSetEnrollmentPolicyBudgetIsNegotiable(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	// A cap of one period: ARTTUE charges it, SWIM does not.
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "ARTTUE", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
	}); err != nil {
		t.Fatalf("place ARTTUE: %v", err)
	}

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: false, PAccept: nil,
	}); err != nil {
		t.Fatalf("place SWIM: %v", err)
	}

	// Charging SWIM as well would occupy 2 of 1.
	err := setPolicy(q, "SWIM", []string{"s1"}, true, true)
	if err == nil {
		t.Fatal("charging a student past their cap was accepted silently")
	}

	expectCodes(t, err, "budget:s1")

	// Refused means unchanged: the raise undoes the update.
	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1' AND counts_toward_budget`); n != 0 {
		t.Fatalf("a refused policy change was applied anyway")
	}

	// Naming the code accepts it, and then it stands.
	if err := setPolicy(q, "SWIM", []string{"s1"}, true, true, "budget:s1"); err != nil {
		t.Fatalf("accepted policy change still refused: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1' AND counts_toward_budget`); n != 1 {
		t.Fatalf("accepted policy change was not applied")
	}
}

// Turning the bit off can only relax the sum, so it never reports
// anything — without the function having to detect the direction.
func TestSetEnrollmentPolicyRelaxingNeverViolates(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
	}); err != nil {
		t.Fatalf("place: %v", err)
	}

	// Over the cap only because of ARTTUE, accepted at placement.
	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "ARTTUE", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true,
		PAccept: []string{"budget:s1"},
	}); err != nil {
		t.Fatalf("place over cap with accept: %v", err)
	}

	// Stopping one of them charging brings the student back under,
	// and needs no accept.
	if err := setPolicy(q, "ARTTUE", []string{"s1"}, true, false); err != nil {
		t.Fatalf("relaxing change refused: %v", err)
	}
}

// A student who is not enrolled cannot be re-policied: silently
// inserting would turn a mistyped id into a placement nobody asked
// for.
func TestSetEnrollmentPolicyIsNotAnUpsert(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: false, PAccept: nil,
	}); err != nil {
		t.Fatalf("place: %v", err)
	}

	// s2 holds no SWIM seat.
	err := setPolicy(q, "SWIM", []string{"s1", "s2"}, false, false)
	expectState(t, err, "P0002")

	// And s1's bits are untouched, the whole call having failed.
	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1' AND student_droppable`); n != 1 {
		t.Fatalf("a failed call changed rows anyway")
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments WHERE course_id = 'SWIM'`); n != 1 {
		t.Fatalf("a failed call inserted a row")
	}
}

// Every declarative administrator write states the arrangement it
// wants, so saving the same arrangement twice must be a no-op — even
// when the state it describes is one that violates a negotiable rule
// and was accepted at the time.
//
// The failure this pins is not theoretical. An administrator forces a
// student over their budget cap, accepting the violation deliberately.
// Later they open the same form to change something else, or click
// Save twice, and the second write re-judges a rule whose inputs it
// did not move — so it comes back asking them to accept, again, a
// decision they already made, in order to complete a write that would
// have changed nothing.
func TestResavingTheSamePolicyIsANoOp(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	for _, course := range []string{"ARTTUE", "SWIM"} {
		if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: []string{"s1"},
			PStudentDroppable: true, PCountsTowardBudget: false, PAccept: nil,
		}); err != nil {
			t.Fatalf("place %s: %v", course, err)
		}
	}

	// Deliberately over the cap, accepted once.
	if err := setPolicy(q, "ARTTUE", []string{"s1"}, true, true); err != nil {
		t.Fatalf("the first charge should be inside the cap: %v", err)
	}

	if err := setPolicy(q, "SWIM", []string{"s1"}, true, true, "budget:s1"); err != nil {
		t.Fatalf("accepted over-cap charge refused: %v", err)
	}

	// The same form again, with no accept list, because the
	// administrator is not being asked to decide anything new.
	if err := setPolicy(q, "SWIM", []string{"s1"}, true, true); err != nil {
		t.Fatalf("re-saving an unchanged policy was refused: %v", err)
	}

	// And it is still what it was.
	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1'
			AND student_droppable AND counts_toward_budget`); n != 1 {
		t.Fatalf("the no-op save changed the row, matching = %d", n)
	}

	// A no-op save still refuses to name an enrollment that is not
	// there: skipping the update must not skip the existence check.
	if err := setPolicy(q, "SWIM", []string{"s1", "s2"}, true, true); err == nil {
		t.Fatal("a policy naming a student with no such enrollment was accepted")
	}
}

// The same guard, for the grade's budget cap.
func TestResavingTheSameBudgetCapIsANoOp(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	ctx := context.Background()

	for _, course := range []string{"ARTTUE", "SWIM"} {
		if err := q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: []string{"s1"},
			PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
		}); err != nil {
			t.Fatalf("place %s: %v", course, err)
		}
	}

	// A cap the student is already over, accepted once.
	if err := q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
		GradeID: "OPEN", MaxBudgetedPeriods: pgInt8(1), Accept: []string{"budget:s1"},
	}); err != nil {
		t.Fatalf("accepted cap refused: %v", err)
	}

	// Saving the same cap again asks nothing of the administrator.
	if err := q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
		GradeID: "OPEN", MaxBudgetedPeriods: pgInt8(1), Accept: []string{},
	}); err != nil {
		t.Fatalf("re-saving an unchanged cap was refused: %v", err)
	}

	// Moving it still judges, so the guard is not a way to smuggle a
	// change past the rules.
	if err := q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
		GradeID: "OPEN", MaxBudgetedPeriods: pgInt8(0), Accept: []string{},
	}); err == nil {
		t.Fatal("a cap change that strands a student was accepted silently")
	}

	// And a grade that does not exist is still refused, guard or not.
	if err := q.SetMaxBudgetedPeriods(ctx, db.SetMaxBudgetedPeriodsParams{
		GradeID: "NOSUCH", MaxBudgetedPeriods: pgInt8(1), Accept: []string{},
	}); err == nil {
		t.Fatal("a cap was set on a grade that does not exist")
	}
}

// Accepting is per violation, not "override everything".
//
// The dialog puts the codes the server named to the administrator and
// sends back what they confirmed, so a partial confirmation must
// refuse exactly the remainder — and a violation that appeared between
// the two attempts, which nobody was shown, must still stop the write.
// That is the whole reason accept takes codes rather than a boolean.
//
// Nothing exercised this path: every existing test accepts all of the
// codes or none of them, and both of those pass against a filter that
// ignores its argument entirely.
func TestAcceptingSomeViolationsRefusesTheRest(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	ctx := context.Background()

	// A student who breaks two rules at once by taking SWIM: it
	// clashes with what they already hold, and it puts them over the
	// cap.
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	if err := q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID: "ARTMON", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
	}); err != nil {
		t.Fatalf("place ARTMON: %v", err)
	}

	// Everything refused, and the codes are the two we expect.
	err := q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: nil,
	})

	codes := refusedCodes(t, err)
	if len(codes) < 2 {
		t.Fatalf("expected at least two violations to accept between, got %v", codes)
	}

	// Accept exactly one of them. The other must still refuse the
	// write, and the refusal must name only the one not accepted.
	accepted := codes[0]
	remaining := codes[1:]

	err = q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true,
		PAccept: []string{accepted},
	})
	if err == nil {
		t.Fatalf("accepting only %q let the whole write through", accepted)
	}

	still := refusedCodes(t, err)
	if slices.Contains(still, accepted) {
		t.Errorf("the accepted code %q came back unaccepted: %v", accepted, still)
	}

	if !slices.Equal(still, remaining) {
		t.Errorf("refused %v, want exactly the unaccepted remainder %v", still, remaining)
	}

	// And nothing was written by the partly-accepted attempt.
	if n := count(t, pool,
		`SELECT count(*) FROM enrollments WHERE course_id = 'SWIM'`); n != 0 {
		t.Fatalf("a partly-accepted write placed %d row(s)", n)
	}

	// Accepting all of them lets it through, so the remainder really
	// was the only thing standing in the way.
	if err := q.PlaceEnrollments(ctx, db.PlaceEnrollmentsParams{
		PCourseID: "SWIM", PStudentIds: []string{"s1"},
		PStudentDroppable: true, PCountsTowardBudget: true, PAccept: codes,
	}); err != nil {
		t.Fatalf("accepting every code was still refused: %v", err)
	}
}

// refusedCodes reads the codes out of a YKV01, sorted so that
// comparisons do not depend on the order the schema built them in.
//
// Distinct from violationCodes, which asks the read model what a
// prospective enrollment would break. This one reads what a write
// actually refused, which is the thing the accept protocol answers.
func refusedCodes(t *testing.T, err error) []string {
	t.Helper()

	if err == nil {
		t.Fatal("expected a violation, got success")
	}

	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != "YKV01" {
		t.Fatalf("expected YKV01, got %v", err)
	}

	var payload []struct {
		Code string `json:"code"`
	}

	if err := json.Unmarshal([]byte(pgErr.Detail), &payload); err != nil {
		t.Fatalf("the YKV01 payload did not decode: %v (%s)", err, pgErr.Detail)
	}

	codes := make([]string, len(payload))
	for i, v := range payload {
		codes[i] = v.Code
	}

	slices.Sort(codes)

	return codes
}

// The droppable bit is no rule's input, so moving it must judge
// nothing — even for a student whose enrollments are over an accepted
// cap.
//
// The scoping has to be on the transition, not on the statement's
// reach. Judging every row the UPDATE touched conflates three
// different things: what the write changed, what the budget rule
// reads, and whether the target value happens to be TRUE. A student
// held over their cap by an administrator who accepted it would then
// be unturnable into a forced placement, because the unrelated flip
// would re-run the budget rule and refuse.
func TestSetEnrollmentPolicyDoesNotJudgeADroppableOnlyFlip(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	// A cap of one period, and three charging enrollments accepted
	// over it.
	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	for _, course := range []string{"SWIM", "ARTTUE", "CLUB"} {
		accept := []string{"budget:s1"}
		if course == "SWIM" {
			accept = nil
		}

		if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: []string{"s1"},
			PStudentDroppable: true, PCountsTowardBudget: true,
			PAccept: accept,
		}); err != nil {
			t.Fatalf("place %s: %v", course, err)
		}
	}

	// Turn one of them into a placement the student may not drop.
	// The budget bit does not move, so the budget is not this write's
	// business.
	if err := setPolicy(q, "SWIM", []string{"s1"}, false, true); err != nil {
		t.Fatalf("a droppable-only flip was refused: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollments
		WHERE course_id = 'SWIM' AND student_id = 's1'
			AND NOT student_droppable AND counts_toward_budget`); n != 1 {
		t.Fatalf("the flip did not take effect")
	}

	// The other direction of the same rule: a genuine FALSE -> TRUE
	// transition is still judged, so the scoping did not simply turn
	// the rule off.
	if err := setPolicy(q, "SWIM", []string{"s1"}, false, false); err != nil {
		t.Fatalf("turning the bit off was refused: %v", err)
	}

	err := setPolicy(q, "SWIM", []string{"s1"}, false, true)
	expectCodes(t, err, "budget:s1")
}

// And re-saving a policy a row already holds is a no-op, not a
// re-judgement: the same call twice must behave the same way twice.
func TestSetEnrollmentPolicyIsIdempotent(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)
	seedWrites(t, pool)

	exec(t, pool, `UPDATE grades SET max_budgeted_periods = 1 WHERE id = 'OPEN'`)

	for _, course := range []string{"SWIM", "ARTTUE"} {
		accept := []string{"budget:s1"}
		if course == "SWIM" {
			accept = nil
		}

		if err := q.PlaceEnrollments(context.Background(), db.PlaceEnrollmentsParams{
			PCourseID: course, PStudentIds: []string{"s1"},
			PStudentDroppable: true, PCountsTowardBudget: true,
			PAccept: accept,
		}); err != nil {
			t.Fatalf("place %s: %v", course, err)
		}
	}

	// Exactly the policy the row already holds.
	for attempt := range 2 {
		if err := setPolicy(q, "SWIM", []string{"s1"}, true, true); err != nil {
			t.Fatalf("attempt %d re-saved an unchanged policy and was refused: %v",
				attempt+1, err)
		}
	}
}
