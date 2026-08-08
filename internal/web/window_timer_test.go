package web //nolint:testpackage

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PaoDevelopers/cca/internal/db"
)

// The timer had no tests at all, which is the wrong amount for the one
// component that decides when twelve hundred open pages learn that
// enrollment has opened.
//
// It is allowed to be unreliable — openness is derived from the two
// timestamps wherever it is read, so a missed tick costs a stale
// button and never a wrong answer — but "unreliable" has to mean
// "occasionally late", not "never fires" or "fires and then stops".

// boundaryDB answers NextWindowBoundary with whatever it is given, and
// counts how many times it was asked.
type boundaryDB struct {
	at    func(call int) pgtype.Timestamptz
	fail  func(call int) bool
	calls chan int
}

func (b *boundaryDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (b *boundaryDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return emptyRows{}, nil
}

func (b *boundaryDB) QueryRow(context.Context, string, ...any) pgx.Row {
	call := len(b.calls)
	b.calls <- call

	if b.fail != nil && b.fail(call) {
		//exhaustruct:ignore
		return boundaryRow{err: errBoundaryRead}
	}

	//exhaustruct:ignore
	return boundaryRow{at: b.at(call)}
}

type boundaryRow struct {
	at  pgtype.Timestamptz
	err error
}

func (r boundaryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	if len(dest) != 1 {
		return errUnexpectedScan
	}

	target, ok := dest[0].(*pgtype.Timestamptz)
	if !ok {
		return errUnexpectedScan
	}

	*target = r.at

	return nil
}

var errUnexpectedScan = errors.New("boundaryRow: unexpected scan target")

var errBoundaryRead = errors.New("boundaryDB: the read failed")

func timerServer(t *testing.T, at func(call int) pgtype.Timestamptz) (*Server, *boundaryDB) {
	t.Helper()

	fake := &boundaryDB{at: at, calls: make(chan int, 16)}
	queries := db.New(fake)
	done := make(chan struct{})

	app := &Server{ //exhaustruct:ignore
		queries:     queries,
		wsHub:       NewWebSocketHub(queries),
		windowTimer: newWindowTimer(done),
	}

	t.Cleanup(func() { close(done) })

	return app, fake
}

// drainWake clears the wake registration itself produced, so that a
// later receive means the timer fired and nothing else.
func drainWake(c *Client) {
	c.takePending()

	select {
	case <-c.wake:
	default:
	}
}

func at(d time.Duration) pgtype.Timestamptz {
	//exhaustruct:ignore
	return pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true}
}

func never() pgtype.Timestamptz {
	//exhaustruct:ignore
	return pgtype.Timestamptz{Valid: false}
}

// The whole job: a bound passes, and every page is told to re-read the
// grades. Nothing else in the system says so — the frames carry no
// state, so a client that hears this and one that does not converge on
// the same answer, but only the one that hears it converges now.
func TestTheTimerBroadcastsWhenABoundPasses(t *testing.T) {
	t.Parallel()

	app, _ := timerServer(t, func(call int) pgtype.Timestamptz {
		if call == 0 {
			return at(10 * time.Millisecond)
		}

		return never()
	})

	client := newWSClient(app.wsHub, nil, "s1")
	app.wsHub.register(client)
	drainWake(client)

	app.rearmWindowTimer(context.Background())

	deadline := time.After(2 * time.Second)

	for {
		select {
		case <-deadline:
			t.Fatal("the boundary passed and nothing was broadcast")
		case <-client.wake:
			if pending := client.takePending(); len(pending) > 0 {
				if pending[0] != "invalidate_grades" {
					t.Fatalf("broadcast %q, want invalidate_grades", pending[0])
				}

				return
			}
		}
	}
}

// Bounds are not consumed by passing: the next one is usually another
// grade's opening, or this grade's closing. A timer that fired once
// and stopped would leave every later boundary silent, and the failure
// would look like "the window did not close" rather than like a timer.
func TestTheTimerRearmsAfterFiring(t *testing.T) {
	t.Parallel()

	app, fake := timerServer(t, func(call int) pgtype.Timestamptz {
		if call < 2 {
			return at(5 * time.Millisecond)
		}

		return never()
	})

	app.rearmWindowTimer(context.Background())

	// Three reads: the arming one, the one after the first firing, and
	// the one after the second.
	deadline := time.After(2 * time.Second)

	for range 3 {
		select {
		case <-fake.calls:
		case <-deadline:
			t.Fatal("the timer stopped asking for the next boundary")
		}
	}
}

// Every bound behind us is the ordinary state outside a selection
// season, and it must simply arm nothing rather than spin.
func TestTheTimerArmsNothingWhenNoBoundIsAhead(t *testing.T) {
	t.Parallel()

	app, _ := timerServer(t, func(int) pgtype.Timestamptz { return never() })

	app.rearmWindowTimer(context.Background())

	app.windowTimer.mu.Lock()
	defer app.windowTimer.mu.Unlock()

	if app.windowTimer.timer != nil {
		t.Error("a timer was armed with no boundary ahead")
	}
}

// A bound that passed while the boundary was being read gives a
// non-positive delay. Firing immediately is correct — the boundary
// really has passed — and is the case a naive guard would drop.
func TestABoundThatHasJustPassedFiresAtOnce(t *testing.T) {
	t.Parallel()

	app, _ := timerServer(t, func(call int) pgtype.Timestamptz {
		if call == 0 {
			return at(-time.Second)
		}

		return never()
	})

	client := newWSClient(app.wsHub, nil, "s1")
	app.wsHub.register(client)
	drainWake(client)

	app.rearmWindowTimer(context.Background())

	select {
	case <-client.wake:
	case <-time.After(2 * time.Second):
		t.Fatal("a boundary already in the past never fired")
	}
}

// Re-arming cancels the pending timer rather than leaving it to fire.
//
// Every grade write calls this, so an afternoon of editing would
// otherwise leave one live timer per write, each still holding the
// bound it was armed at — and each firing at a time that is no longer
// a boundary at all. The assertion is that the *old* one stays silent,
// not merely that the field was overwritten: a replaced field with an
// un-stopped timer behind it is exactly the bug.
// In a synctest bubble, because the assertion is a negative one and a
// negative assertion is only as strong as the time it waited.
//
// It used to wait 200 milliseconds of real time for a boundary 30
// milliseconds out: a fifth of a second of test run, to establish
// something much weaker than what is meant — it would have passed
// against a timer that fired a second later. In a bubble the clock is
// fake and only advances when every goroutine is durably blocked, so
// sleeping through an hour of it costs nothing and says an hour.
func TestRearmingCancelsThePendingTimer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The first read arms a timer 30ms out. The second — the
		// re-arm — says there is no boundary ahead, so nothing should
		// fire at all.
		app, _ := timerServer(t, func(call int) pgtype.Timestamptz {
			if call == 0 {
				return at(30 * time.Millisecond)
			}

			return never()
		})

		client := newWSClient(app.wsHub, nil, "s1")
		app.wsHub.register(client)
		drainWake(client)

		app.rearmWindowTimer(context.Background())

		app.windowTimer.mu.Lock()
		armed := app.windowTimer.timer
		app.windowTimer.mu.Unlock()

		if armed == nil {
			t.Fatal("nothing was armed")
		}

		// The bound is withdrawn before it arrives.
		app.rearmWindowTimer(context.Background())

		app.windowTimer.mu.Lock()
		remaining := app.windowTimer.timer
		app.windowTimer.mu.Unlock()

		if remaining != nil {
			t.Error("a timer was left armed with no boundary ahead")
		}

		// An hour of the bubble's clock, which passes instantly. Wait
		// alone would not do: it blocks until the other goroutines
		// are, but the clock moves for a sleep, not for a wait.
		time.Sleep(time.Hour)
		synctest.Wait()

		select {
		case <-client.wake:
			t.Error("the withdrawn boundary fired anyway")
		default:
		}
	})
}

// A server with no timer — which is every test server, and the state
// before startup finishes — must not panic when a write re-arms.
func TestRearmingWithoutATimerIsHarmless(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.rearmWindowTimer(context.Background())
}

// One failed read must not end the season's repainting.
//
// The error path used to log and return, which left the spent timer in
// the field with nothing to replace it. A single pool hiccup on the
// re-arm after the 08:00 opening therefore silenced every boundary
// after it — the "fires and then stops" this file's header rules out —
// until an administrator happened to write a window bound or the
// process was restarted.
func TestAFailedBoundaryReadIsRetried(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		app, fake := timerServer(t, func(int) pgtype.Timestamptz {
			return never()
		})
		// Every read fails, so nothing can be arming except the retry.
		fake.fail = func(int) bool { return true }

		app.rearmWindowTimer(context.Background())

		if len(fake.calls) != 1 {
			t.Fatalf("asked %d times before waiting, want 1",
				len(fake.calls))
		}

		time.Sleep(windowTimerRetry + time.Second)
		synctest.Wait()

		if len(fake.calls) < 2 {
			t.Fatalf("asked %d times across a retry interval; the "+
				"timer gave up after one failure", len(fake.calls))
		}
	})
}

// A retry is not a boundary, so it must not repaint anybody.
func TestARetryDoesNotClaimABoundaryPassed(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		app, fake := timerServer(t, func(int) pgtype.Timestamptz {
			return never()
		})
		fake.fail = func(int) bool { return true }

		client := newWSClient(app.wsHub, nil, "s1")
		app.wsHub.register(client)
		drainWake(client)

		app.rearmWindowTimer(context.Background())

		time.Sleep(3 * windowTimerRetry)
		synctest.Wait()

		select {
		case <-client.wake:
			t.Error("a failed read told every page a boundary passed")
		default:
		}
	})
}

// Our clock running ahead of the database's must not become a spin.
//
// NextWindowBoundary only returns bounds the database still calls
// future. time.Until reads our clock. So while we are ahead by delta,
// the boundary we just fired for comes back with a non-positive delay,
// and firing on it again broadcasts to every open page, re-arms, and
// gets the same answer — for the whole of delta. The fake here holds
// one instant fixed, which is that disagreement made permanent.
func TestABoundaryTheDatabaseStillCallsFutureDoesNotSpin(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// One instant, already behind us, returned for ever.
		stuck := at(-time.Second)
		app, fake := timerServer(t, func(int) pgtype.Timestamptz {
			return stuck
		})

		app.rearmWindowTimer(context.Background())

		const window = 10 * time.Second

		time.Sleep(window)
		synctest.Wait()

		// One pass per backoff, plus the arming read and slack. The
		// number that matters is that it is bounded at all: unguarded
		// this is limited only by how fast a round trip completes.
		if limit := int(window/windowTimerSkewBackoff) + 3; len(fake.calls) > limit {
			t.Errorf("asked %d times in %v, want at most %d; the "+
				"timer is spinning on a boundary that has not passed",
				len(fake.calls), window, limit)
		}
	})
}
