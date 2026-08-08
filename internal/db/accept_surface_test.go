package db_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Which operations take an accept list is the load-bearing claim of
// the whole write protocol:
//
//	An administrator operation takes p_accept if and only if it
//	changes an input to a negotiable rule.
//
// Administrator, because the student operations are the deliberate
// exception: self_enroll, self_drop and self_swap change clash, budget
// and capacity inputs and take no accept list at all. A student
// accepts nothing — they are refused, and told why.
//
// Everything else follows from it. An operation that takes one and
// should not is asking an administrator to confirm something that
// cannot break; one that does not and should is applying a change
// nobody was shown.
//
// The claim was documented and counted by hand, and the count had
// drifted: docs/operations.md said create_course took an accept list,
// which it does not, and omitted set_enrollment_policy, which does.
// Neither is visible from reading either file alone. This test reads
// the schema and states the set, so a function that gains or loses an
// accept list has to be accounted for here — and the comments below
// are where the reason goes.

// The write functions that take p_accept, and why each one may need to
// ask. Sorted, because the test compares sorted.
//
//nolint:gochecknoglobals
var acceptingWrites = []string{
	// Placing a student into a course judges all five rules against
	// them: it is the operation the rules exist for.
	"place_enrollments",

	// Turning counts_toward_budget on can put a student over their
	// cap. The other bit, student_droppable, is nobody's rule input.
	"set_enrollment_policy",

	// Periods move clashes and the budget; grades and legal sexes move
	// eligibility; capacity can leave the course overfull. All of them
	// are judged against the students already enrolled.
	"update_course",

	// The batch form of update_course, and so the same set — per
	// element, scoped to what that element moved.
	"upsert_courses",

	// Lowering a grade's cap can put students already enrolled over
	// it.
	"set_max_budgeted_periods",

	// Moving a student between grades changes which courses they are
	// eligible for and which cap applies; changing their legal sex
	// changes eligibility too.
	"upsert_students",
}

// The helpers that also take p_accept. They are not operations — they
// compute a payload for one — and they are listed so that the check
// below can tell "a helper" from "an operation nobody accounted for".
//
//nolint:gochecknoglobals
var acceptingHelpers = []string{
	"course_enrollee_violations",
	"student_budget_violation",
	"student_enrollment_violations",
	"unaccepted_violations",
}

func TestTheAcceptSurfaceIsExactlyWhatIsDocumented(t *testing.T) {
	t.Parallel()

	names, err := filepath.Glob(filepath.Join("schemas", "*.sql"))
	if err != nil {
		t.Fatalf("list the schema files: %v", err)
	}

	// The signature runs to the closing parenthesis before RETURNS.
	signature := regexp.MustCompile(`(?s)CREATE FUNCTION (\w+)\((.*?)\)\s*\nRETURNS`)

	var found []string

	for _, name := range names {
		content, err := os.ReadFile(name) //#nosec G304 -- fixed test data
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		for _, match := range signature.FindAllStringSubmatch(string(content), -1) {
			if strings.Contains(match[2], "p_accept") {
				found = append(found, match[1])
			}
		}
	}

	if len(found) == 0 {
		t.Fatal("no function was found to take p_accept; the check is vacuous")
	}

	want := slices.Concat(acceptingWrites, acceptingHelpers)
	slices.Sort(want)
	slices.Sort(found)

	if !slices.Equal(found, want) {
		t.Errorf("the accept surface has moved.\n"+
			"  schema has %v\n"+
			"  recorded   %v\n"+
			"An operation that gained an accept list must change an "+
			"input to a negotiable rule, and one that lost an accept "+
			"list must no longer change one. Say which in this file, "+
			"and check docs/operations.md still agrees.", found, want)
	}
}
