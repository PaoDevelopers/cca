package db_test

import (
	"context"
	"testing"
)

// The two manual window levers.
//
// Both read the clock in the statement that writes the bound, which is
// the property under test as much as any of the outcomes below: the
// value must come from the database, because the database is what
// v_grades.is_open is evaluated against. Taking it anywhere else made
// three clocks, and delivered it rounded to the minute, which is how
// opening and closing inside one minute came to write closes_at =
// opens_at and be refused by the CHECK.

// Opening keeps a closing time that is still ahead of us. A schedule
// is something an administrator set on purpose, and starting a window
// early says nothing about when it should stop.
func TestOpenNowKeepsAScheduledClose(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order,
		 opens_at, closes_at) VALUES
		('SCHED', 'Scheduled', NULL, 0, 1,
			now() + interval '2 hours', now() + interval '4 hours')`)

	res, err := q.OpenGradeWindowNow(context.Background(), "SCHED")
	if err != nil {
		t.Fatalf("open now: %v", err)
	}

	if !res.Found || !res.Opened {
		t.Fatalf("found=%v opened=%v, want both true", res.Found, res.Opened)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM v_grades WHERE id = 'SCHED' AND is_open`); n != 1 {
		t.Error("the window did not open")
	}

	// The assertion that matters: the bound the administrator set is
	// still there, and still in the future.
	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'SCHED' AND closes_at > now() + interval '3 hours'`); n != 1 {
		t.Error("opening discarded the scheduled closing time")
	}
}

// A closing time already behind us cannot be kept and opened around:
// the row would say the window shuts before it starts, and the CHECK
// forbids it. Refusing is the honest answer — opening anyway would
// mean silently discarding a bound, which is the behaviour this
// replaced.
func TestOpenNowRefusesWhenTheCloseHasPassed(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order,
		 opens_at, closes_at) VALUES
		('PAST', 'Already ran', NULL, 0, 1,
			now() - interval '4 hours', now() - interval '2 hours')`)

	res, err := q.OpenGradeWindowNow(context.Background(), "PAST")
	if err != nil {
		t.Fatalf("open now: %v", err)
	}

	if !res.Found {
		t.Fatal("the grade exists; found was false")
	}

	if res.Opened {
		t.Error("a window whose closing time has passed was opened anyway")
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'PAST' AND opens_at < now() - interval '3 hours'`); n != 1 {
		t.Error("a refused open still moved opens_at")
	}
}

// found and opened are different answers and the caller distinguishes
// them: one is 404, the other 409.
func TestOpenNowReportsAMissingGradeAsMissing(t *testing.T) {
	t.Parallel()
	_, q := fresh(t)

	res, err := q.OpenGradeWindowNow(context.Background(), "NOSUCH")
	if err != nil {
		t.Fatalf("open now: %v", err)
	}

	if res.Found || res.Opened {
		t.Errorf("found=%v opened=%v, want both false", res.Found, res.Opened)
	}
}

// The regression the levers were moved for. Two presses as close
// together as the process can make them: both must land, and the row
// must satisfy closes_at > opens_at. Through the browser these arrived
// rounded to the minute, so the second write was refused and the
// window stayed open — the failure mode being an administrator told
// "that value is not valid here" about a value they never typed.
func TestOpenThenCloseInTheSameInstantBothLand(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order)
		VALUES ('G', 'Grade', NULL, 0, 1)`)

	ctx := context.Background()

	res, err := q.OpenGradeWindowNow(ctx, "G")
	if err != nil {
		t.Fatalf("open now: %v", err)
	}

	if !res.Opened {
		t.Fatal("the window did not open")
	}

	closed, err := q.CloseGradeWindowNow(ctx, "G")
	if err != nil {
		t.Fatalf("close now: %v", err)
	}

	if closed != 1 {
		t.Fatalf("closed %d rows, want 1", closed)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM v_grades WHERE id = 'G' AND is_open`); n != 0 {
		t.Error("the window is still open after closing it")
	}

	if n := count(t, pool,
		`SELECT count(*) FROM grades WHERE id = 'G' AND closes_at > opens_at`); n != 1 {
		t.Error("the two bounds are not ordered; the clock did not advance between them")
	}
}

// Closing is guarded on v_grades.is_open rather than on a bound
// comparison written out a second time. The difference is not
// academic: guarding on opens_at < now() alone also matches a window
// that opened and closed yesterday, and closing it then overwrote the
// record of when it really ran.
func TestCloseNowLeavesAWindowThatAlreadyRanAlone(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order,
		 opens_at, closes_at) VALUES
		('PAST', 'Already ran', NULL, 0, 1,
			now() - interval '4 hours', now() - interval '2 hours')`)

	closed, err := q.CloseGradeWindowNow(context.Background(), "PAST")
	if err != nil {
		t.Fatalf("close now: %v", err)
	}

	if closed != 0 {
		t.Errorf("closed %d rows; there was nothing open to close", closed)
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'PAST' AND closes_at < now() - interval '1 hour'`); n != 1 {
		t.Error("closing an already-shut window overwrote when it actually closed")
	}
}

// A window merely scheduled to open later is cancelled by clearing
// opens_at, not by closing it. Writing a closing time here would
// produce a grade that closes before it opens.
func TestCloseNowIgnoresAWindowThatHasNotOpened(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order,
		 opens_at) VALUES
		('FUTURE', 'Not yet', NULL, 0, 1, now() + interval '6 hours')`)

	closed, err := q.CloseGradeWindowNow(context.Background(), "FUTURE")
	if err != nil {
		t.Fatalf("close now: %v", err)
	}

	if closed != 0 {
		t.Errorf("closed %d rows; the window had not opened", closed)
	}

	if n := count(t, pool,
		`SELECT count(*) FROM grades WHERE id = 'FUTURE' AND closes_at IS NULL`); n != 1 {
		t.Error("a window that had not opened was given a closing time")
	}
}

// Closing leaves opens_at alone, so the card can still say when the
// window ran rather than only that nothing is happening.
func TestCloseNowKeepsTheRecordOfWhenTheWindowRan(t *testing.T) {
	t.Parallel()
	pool, q := fresh(t)

	exec(t, pool, `INSERT INTO grades
		(id, name, max_budgeted_periods, min_distinct_categories, sort_order,
		 opens_at) VALUES
		('G', 'Grade', NULL, 0, 1, now() - interval '3 hours')`)

	if _, err := q.CloseGradeWindowNow(context.Background(), "G"); err != nil {
		t.Fatalf("close now: %v", err)
	}

	if n := count(t, pool, `SELECT count(*) FROM grades
		WHERE id = 'G' AND opens_at < now() - interval '2 hours'`); n != 1 {
		t.Error("closing moved or cleared opens_at")
	}
}
