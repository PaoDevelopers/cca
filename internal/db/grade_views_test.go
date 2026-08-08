package db_test

import (
	"context"
	"slices"
	"testing"
)

// The grade read models: derived window openness and requirement
// aggregation, read through the same queries production uses.

func TestWindowOpennessDerivation(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()

	// Every window configuration, including both boundary
	// instants. Seed and boundary read share one transaction,
	// so now() is frozen and ATOPEN/ATCLOSE sit exactly on their
	// bounds when judged, pinning the half-open interval
	// [opens_at, closes_at); a read in a later transaction would
	// see a moved now() and stop testing the boundary at all.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
		('BOTHNULL', 'Both null', NULL, NULL, NULL, 0, 1),
		('OPEN', 'Open now', now() - interval '1 hour', now() + interval '1 hour', NULL, 0, 2),
		('PAST', 'Already closed', now() - interval '2 hours', now() - interval '1 hour', NULL, 0, 3),
		('FUTURE', 'Not yet open', now() + interval '1 hour', now() + interval '2 hours', NULL, 0, 4),
		('NOEND', 'Open, never closes', now() - interval '1 hour', NULL, NULL, 0, 5),
		('NOSTART', 'Closes without opening', NULL, now() + interval '1 hour', NULL, 0, 6),
		('ATOPEN', 'Opens exactly now', now(), now() + interval '1 hour', NULL, 0, 7),
		('ATCLOSE', 'Closes exactly now', now() - interval '1 hour', now(), NULL, 0, 8)`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var atOpen, atClose bool
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT is_open FROM v_grades WHERE id = 'ATOPEN'),
		(SELECT is_open FROM v_grades WHERE id = 'ATCLOSE')`).
		Scan(&atOpen, &atClose); err != nil {
		t.Fatalf("boundary read: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	grades, err := q.GetGrades(context.Background())
	if err != nil {
		t.Fatalf("GetGrades: %v", err)
	}

	want := map[string]bool{
		"BOTHNULL": false,
		"OPEN":     true,
		"PAST":     false,
		"FUTURE":   false,
		"NOEND":    true,
		"NOSTART":  false,
	}

	if len(grades) != 8 {
		t.Fatalf("v_grades carries %d grades, want 8", len(grades))
	}

	for _, g := range grades {
		if !g.IsOpen.Valid {
			t.Fatalf("%s: is_open must never be NULL", g.ID)
		}

		if w, ok := want[g.ID]; ok && g.IsOpen.Bool != w {
			t.Fatalf("%s: is_open = %v, want %v", g.ID, g.IsOpen.Bool, w)
		}
	}

	if !atOpen {
		t.Fatal("ATOPEN: opens_at must be inclusive")
	}

	if atClose {
		t.Fatal("ATCLOSE: closes_at must be exclusive")
	}
}

func TestRequirementAggregation(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()

	exec(t, pool, `INSERT INTO categories (id, name) VALUES
			('SPORT', 'Sports'), ('ART', 'Art'), ('CULTURE', 'Culture');
		INSERT INTO grades (id, name, opens_at, closes_at, max_budgeted_periods, min_distinct_categories, sort_order) VALUES
			('OPEN', 'Open', NULL, NULL, NULL, 0, 1),
			('PAST', 'Past', NULL, NULL, NULL, 0, 2)`)

	// Members inserted RAW and out of id order:
	// the view must sort, not echo insertion order —
	// and the write function's own DISTINCT would normalize the
	// order, which is why this seed bypasses it.
	var two int64
	if err := pool.QueryRow(ctx, `INSERT INTO grade_requirement_groups
		(grade_id, min_period_count) VALUES ('OPEN', 3)
		RETURNING id`).Scan(&two); err != nil {
		t.Fatalf("raw requirement: %v", err)
	}

	exec(t, pool, `INSERT INTO grade_requirement_group_categories
		(requirement_category_id, category_id) VALUES ($1, 'SPORT'), ($1, 'ART')`, two)

	var one int64
	if err := pool.QueryRow(ctx, `INSERT INTO grade_requirement_groups
		(grade_id, min_period_count) VALUES ('OPEN', 1)
		RETURNING id`).Scan(&one); err != nil {
		t.Fatalf("raw requirement: %v", err)
	}

	exec(t, pool, `INSERT INTO grade_requirement_group_categories
		(requirement_category_id, category_id) VALUES ($1, 'CULTURE')`, one)

	// A memberless requirement cannot be created through the write
	// layer; seed one raw so the view's empty-array contract stays
	// pinned regardless.
	var none int64
	if err := pool.QueryRow(ctx, `INSERT INTO grade_requirement_groups
		(grade_id, min_period_count) VALUES ('PAST', 0)
		RETURNING id`).Scan(&none); err != nil {
		t.Fatalf("raw requirement: %v", err)
	}

	reqs, err := q.GetGradeRequirements(ctx)
	if err != nil {
		t.Fatalf("GetGradeRequirements: %v", err)
	}

	if len(reqs) != 3 {
		t.Fatalf("%d requirement rows, want 3", len(reqs))
	}

	byID := map[int64][]string{}
	openCount := 0

	for _, r := range reqs {
		byID[r.ID] = r.CategoryIds
		if r.GradeID == "OPEN" {
			openCount++
		}
	}

	if !slices.Equal(byID[two], []string{"ART", "SPORT"}) {
		t.Fatalf("members = %v, want id-ordered [ART SPORT]", byID[two])
	}

	if !slices.Equal(byID[one], []string{"CULTURE"}) {
		t.Fatalf("single member = %v", byID[one])
	}

	if byID[none] == nil || len(byID[none]) != 0 {
		t.Fatalf("no members must read as an empty array, not NULL: %#v", byID[none])
	}

	if openCount != 2 {
		t.Fatalf("a grade may hold several requirements; got %d", openCount)
	}
}
