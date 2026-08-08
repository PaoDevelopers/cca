package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Display order is declarative: the caller names the whole
// arrangement. Creation appends, deletion is a plain delete, and
// neither maintains an invariant beyond "sort_order is a number".

// gradeOrder returns the display order as one string, e.g. "A,B,C",
// read the way production reads it: sort_order, then id.
func gradeOrder(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var s pgtype.Text
	if err := pool.QueryRow(context.Background(),
		`SELECT string_agg(id, ',' ORDER BY sort_order, id) FROM grades`).
		Scan(&s); err != nil {
		t.Fatalf("order: %v", err)
	}

	return s.String
}

func TestGradeCreationAppends(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	newGrade := func(id string, limit pgtype.Int8, minCategories int64) {
		t.Helper()

		if err := q.NewGrade(ctx, db.NewGradeParams{
			ID: id, Name: "Grade " + id,
			MaxBudgetedPeriods: limit, MinDistinctCategories: minCategories,
		}); err != nil {
			t.Fatalf("new grade %s: %v", id, err)
		}
	}

	newGrade("A", pgtype.Int8{}, 0)
	newGrade("B", pgtype.Int8{Int64: 8, Valid: true}, 2)
	newGrade("C", pgtype.Int8{}, 0)

	if o := gradeOrder(t, pool); o != "A,B,C" {
		t.Fatalf("creation must append at the end: %s", o)
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'B' AND max_budgeted_periods = 8
			AND min_distinct_categories = 2`); n != 1 {
		t.Fatal("creation must carry its parameters")
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'A' AND opens_at IS NULL AND closes_at IS NULL`); n != 1 {
		t.Fatal("a new grade starts with its window unset (closed)")
	}
}

func TestGradeOrderIsDeclarative(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	for _, id := range []string{"A", "B", "C", "D"} {
		if err := q.NewGrade(ctx, db.NewGradeParams{
			ID: id, Name: "Grade " + id,
		}); err != nil {
			t.Fatalf("new grade %s: %v", id, err)
		}
	}

	if err := q.SetGradeOrder(ctx, []string{"C", "A", "D", "B"}); err != nil {
		t.Fatalf("set order: %v", err)
	}

	if o := gradeOrder(t, pool); o != "C,A,D,B" {
		t.Fatalf("the stored order must be the order named: %s", o)
	}

	// Idempotent: the same arrangement applied twice is the same
	// arrangement. This is the property the relative move lacked.
	if err := q.SetGradeOrder(ctx, []string{"C", "A", "D", "B"}); err != nil {
		t.Fatalf("set order again: %v", err)
	}

	if o := gradeOrder(t, pool); o != "C,A,D,B" {
		t.Fatalf("reordering must be idempotent: %s", o)
	}

	// The positions themselves are 1..N, so a client may show
	// "3 of 4" without consulting anything else.
	if n := count(t, pool,
		`SELECT count(*) FROM grades WHERE sort_order NOT BETWEEN 1 AND 4`); n != 0 {
		t.Fatal("a declared order must assign 1..N")
	}
}

func TestGradeOrderMustBeTotal(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	for _, id := range []string{"A", "B", "C"} {
		if err := q.NewGrade(ctx, db.NewGradeParams{
			ID: id, Name: "Grade " + id,
		}); err != nil {
			t.Fatalf("new grade %s: %v", id, err)
		}
	}

	// A partial order is not an order: the rows left unnamed would
	// keep positions the administrator did not choose.
	expectState(t, q.SetGradeOrder(ctx, []string{"A", "B"}), "22023")

	// Naming a row twice is ambiguous rather than partial. The
	// interesting case is the one where the arithmetic works out
	// anyway — every grade is named, so the count of rows moved is
	// right, and only an explicit check can see that the list was
	// never an order at all.
	expectState(t, q.SetGradeOrder(ctx, []string{"A", "B", "C", "C"}), "22023")
	expectState(t, q.SetGradeOrder(ctx, []string{"A", "B", "B"}), "22023")

	// A name that is not a grade leaves a real grade unplaced.
	expectState(t, q.SetGradeOrder(ctx, []string{"A", "B", "NOPE"}), "22023")

	// Every rejection is total: the creation order still stands.
	if o := gradeOrder(t, pool); o != "A,B,C" {
		t.Fatalf("a rejected order must move nothing: %s", o)
	}
}

func TestGradeDeletion(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	for _, id := range []string{"A", "B", "C"} {
		if err := q.NewGrade(ctx, db.NewGradeParams{
			ID: id, Name: "Grade " + id,
		}); err != nil {
			t.Fatalf("new grade %s: %v", id, err)
		}
	}

	n, err := q.DeleteGrade(ctx, "B")
	if err != nil || n != 1 {
		t.Fatalf("delete: %v, %d rows", err, n)
	}

	// The gap left behind is not repaired, and does not need to be:
	// the order is still A then C.
	if o := gradeOrder(t, pool); o != "A,C" {
		t.Fatalf("deletion must preserve the relative order: %s", o)
	}

	// Deleting what is not there is reported by the row count,
	// not by an exception: it is plain DML.
	n, err = q.DeleteGrade(ctx, "NOPE")
	if err != nil || n != 0 {
		t.Fatalf("deleting a missing grade must affect no rows: %v, %d", err, n)
	}

	exec(t, pool, `INSERT INTO students (id, name, grade_id, legal_sex)
		VALUES ('s1', 'Student', 'A', 'F')`)

	_, err = q.DeleteGrade(ctx, "A")
	expectState(t, err, "23503")
}

// Ties are possible (two creations racing pick the same position)
// and must still produce one definite order rather than an arbitrary
// one that changes between reads.
func TestOrderTiesBreakByID(t *testing.T) {
	t.Parallel()
	pool, _ := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, min_distinct_categories, sort_order)
		VALUES ('B', 'Second', 0, 7), ('A', 'First', 0, 7)`)

	if o := gradeOrder(t, pool); o != "A,B" {
		t.Fatalf("a tie must break by id: %s", o)
	}
}

func TestPeriodOrdering(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	ctx := context.Background()
	for _, id := range []string{"MON1", "TUE1", "WED1", "THU1"} {
		if err := q.NewPeriod(ctx, db.NewPeriodParams{
			ID: id, Name: "Period " + id,
		}); err != nil {
			t.Fatalf("new period %s: %v", id, err)
		}
	}

	if err := q.SetPeriodOrder(ctx,
		[]string{"THU1", "MON1", "WED1", "TUE1"}); err != nil {
		t.Fatalf("set order: %v", err)
	}

	var order pgtype.Text
	if err := pool.QueryRow(ctx,
		`SELECT string_agg(id, ',' ORDER BY sort_order, id) FROM periods`).
		Scan(&order); err != nil {
		t.Fatalf("order: %v", err)
	}

	if order.String != "THU1,MON1,WED1,TUE1" {
		t.Fatalf("the stored order must be the order named: %s", order.String)
	}

	expectState(t, q.SetPeriodOrder(ctx, []string{"MON1"}), "22023")
	expectState(t, q.SetPeriodOrder(ctx,
		[]string{"MON1", "TUE1", "WED1", "THU1", "THU1"}), "22023")

	n, err := q.DeletePeriod(ctx, "MON1")
	if err != nil || n != 1 {
		t.Fatalf("delete: %v, %d rows", err, n)
	}

	n, err = q.DeletePeriod(ctx, "MON1")
	if err != nil || n != 0 {
		t.Fatalf("deleting a missing period must affect no rows: %v, %d", err, n)
	}
}
