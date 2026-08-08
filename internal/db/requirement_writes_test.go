package db_test

import (
	"context"
	"encoding/json/v2"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Requirement writes are declarative: a grade's requirements are set
// as a whole, so the operation is idempotent and there is no
// read-modify-write for a caller to get wrong.

// requirement is one element of the JSONB parameter.
type requirement struct {
	MinPeriodCount int64    `json:"min_period_count"`
	CategoryIds    []string `json:"category_ids"`
}

func setRequirements(t *testing.T, q *db.Queries, grade string, reqs ...requirement) error {
	t.Helper()

	if reqs == nil {
		reqs = []requirement{}
	}

	payload, err := json.Marshal(reqs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return q.SetGradeRequirements(context.Background(),
		db.SetGradeRequirementsParams{PGradeID: grade, PRequirements: payload})
}

// requirementID finds the requirement of a grade with the given
// minimum. Ids are internal and churn on every edit, so tests that
// need one look it up rather than remembering it.
func requirementID(t *testing.T, pool *pgxpool.Pool, grade string, minPeriods int64) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM grade_requirement_groups
		WHERE grade_id = $1 AND min_period_count = $2`,
		grade, minPeriods).Scan(&id); err != nil {
		t.Fatalf("requirement %s/%d: %v", grade, minPeriods, err)
	}

	return id
}

func TestRequirementsAreSetAsAWhole(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO categories (id, name)
			VALUES ('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', NULL, 0, 1)`)

	ctx := context.Background()

	// Members land deduplicated and ordered.
	if err := setRequirements(t, q, "Y9",
		requirement{3, []string{"SPORT", "ART", "SPORT"}}); err != nil {
		t.Fatalf("set: %v", err)
	}

	reqs, err := q.GetGradeRequirements(ctx)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("reqs %v, err %v", reqs, err)
	}

	if reqs[0].MinPeriodCount != 3 ||
		!slices.Equal(reqs[0].CategoryIds, []string{"ART", "SPORT"}) {
		t.Fatalf("members must land deduplicated and ordered: %+v", reqs[0])
	}

	// Setting a different arrangement replaces the old one whole:
	// no residue from what was there before.
	if err := setRequirements(t, q, "Y9",
		requirement{1, []string{"SPORT"}},
		requirement{2, []string{"ART"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	reqs, err = q.GetGradeRequirements(ctx)
	if err != nil || len(reqs) != 2 {
		t.Fatalf("replacement must leave exactly the new set: %v, %v", reqs, err)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM grade_requirement_group_categories`); n != 2 {
		t.Fatalf("stale members must go with their requirement; %d rows", n)
	}

	// Idempotent, like every declarative write.
	if err := setRequirements(t, q, "Y9",
		requirement{1, []string{"SPORT"}},
		requirement{2, []string{"ART"}}); err != nil {
		t.Fatalf("replace again: %v", err)
	}

	again, err := q.GetGradeRequirements(ctx)
	if err != nil || len(again) != 2 {
		t.Fatalf("setting the same arrangement twice must change nothing: %v", err)
	}

	// Clearing is naming none of them.
	if err := setRequirements(t, q, "Y9"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if n := count(t, pool, `SELECT (SELECT count(*) FROM grade_requirement_groups)
		+ (SELECT count(*) FROM grade_requirement_group_categories)`); n != 0 {
		t.Fatalf("clearing must remove the requirements and their members; %d rows", n)
	}
}

func TestRequirementWritesRejectMalformed(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO categories (id, name)
			VALUES ('SPORT', 'Sports'), ('ART', 'Art');
		INSERT INTO grades (id, name, max_budgeted_periods, min_distinct_categories, sort_order)
			VALUES ('Y9', 'Year 9', NULL, 0, 1)`)

	ctx := context.Background()

	// A requirement over no categories is unsatisfiable or inert:
	// malformed either way.
	expectState(t, setRequirements(t, q, "Y9", requirement{3, nil}), "22023")

	// An unknown grade and an unknown category are the same kind of
	// mistake — a name that is not there — and now report the same
	// way. The category used to surface as the insert's 23503, which
	// named a foreign key constraint rather than the category.
	expectState(t, setRequirements(t, q, "Y99",
		requirement{3, []string{"SPORT"}}), "P0002")
	expectState(t, setRequirements(t, q, "Y9",
		requirement{3, []string{"NOPE"}}), "P0002")

	// A rejection is total: an earlier well-formed element in the
	// same call must not survive.
	expectState(t, setRequirements(t, q, "Y9",
		requirement{1, []string{"SPORT"}},
		requirement{2, nil}), "22023")

	if n := count(t, pool, `SELECT count(*) FROM grade_requirement_groups`); n != 0 {
		t.Fatalf("a rejected arrangement must store nothing; %d rows", n)
	}

	// A category named by a requirement cannot be deleted from
	// under it.
	if err := setRequirements(t, q, "Y9",
		requirement{3, []string{"ART"}}); err != nil {
		t.Fatalf("set: %v", err)
	}

	_, err := q.DeleteCategory(ctx, "ART")
	expectState(t, err, "23503")
}
