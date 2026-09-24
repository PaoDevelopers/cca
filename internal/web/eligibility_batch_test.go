package web //nolint:testpackage

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

var errBoom = errors.New("boom")

// A loader that blocks until released and reports which query, in
// start order, produced each answer.
type gatedLoader struct {
	started atomic.Int64
	running atomic.Int64
	peak    atomic.Int64
	release chan struct{}
	// Whether each query's context was already done when it finished,
	// to check none was cancelled by a caller going away.
	mu        sync.Mutex
	cancelled []bool
}

func newGatedLoader() *gatedLoader {
	//exhaustruct:ignore
	return &gatedLoader{release: make(chan struct{})}
}

func (g *gatedLoader) load(ctx context.Context, studentID string) (eligibility, error) {
	n := g.started.Add(1)

	now := g.running.Add(1)
	for {
		peak := g.peak.Load()
		if now <= peak || g.peak.CompareAndSwap(peak, now) {
			break
		}
	}

	<-g.release
	g.running.Add(-1)

	g.mu.Lock()
	g.cancelled = append(g.cancelled, ctx.Err() != nil)
	g.mu.Unlock()

	// The answer names the query that produced it.
	return eligibility{studentID: {{Detail: strconv.FormatInt(n, 10)}}}, nil
}

func queryOf(t *testing.T, out eligibility, studentID string) string {
	t.Helper()

	v := out[studentID]
	if len(v) != 1 {
		t.Fatalf("answer for %s has %d entries, want 1", studentID, len(v))
	}

	return v[0].Detail
}

// Every tab of one student asking at once costs two queries, not one
// per tab: the one already running and the one they all share.
func TestEligibilityBatchSharesOneQueryPerStudent(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var b eligibilityBatcher

		g := newGatedLoader()

		first := make(chan eligibility, 1)

		go func() {
			out, _ := b.get(t.Context(), "s1", g.load)
			first <- out
		}()

		synctest.Wait()

		const tabs = 50

		results := make(chan eligibility, tabs)

		for range tabs {
			go func() {
				out, err := b.get(t.Context(), "s1", g.load)
				if err != nil {
					t.Errorf("get: %v", err)
				}

				results <- out
			}()
		}

		synctest.Wait()

		if got := g.started.Load(); got != 1 {
			t.Fatalf("%d queries started while the first ran, want 1", got)
		}

		close(g.release)

		if got := queryOf(t, <-first, "s1"); got != "1" {
			t.Errorf("first caller got query %s, want 1", got)
		}

		for range tabs {
			// Each of them arrived while query 1 ran, so none of
			// them may be answered by it.
			if got := queryOf(t, <-results, "s1"); got != "2" {
				t.Errorf("a waiting caller got query %s, want 2", got)
			}
		}

		if got := g.started.Load(); got != 2 {
			t.Errorf("%d queries in all, want 2", got)
		}

		b.mu.Lock()
		left := len(b.flights)
		b.mu.Unlock()

		if left != 0 {
			t.Errorf("%d students still have flights after all answered", left)
		}
	})
}

// A caller that gives up gets its own context's error and nothing
// else changes: the query it was waiting on still runs, uncancelled,
// and still answers the callers that stayed.
func TestEligibilityBatchCallerLeavingDoesNotCancelTheQuery(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var b eligibilityBatcher

		g := newGatedLoader()

		leaving, leave := context.WithCancel(t.Context())

		gone := make(chan error, 1)

		go func() {
			_, err := b.get(leaving, "s1", g.load)
			gone <- err
		}()

		stayed := make(chan eligibility, 1)

		go func() {
			out, _ := b.get(t.Context(), "s1", g.load)
			stayed <- out
		}()

		synctest.Wait()
		leave()

		if err := <-gone; !errors.Is(err, context.Canceled) {
			t.Fatalf("the caller that left got %v, want context.Canceled", err)
		}

		close(g.release)

		out := <-stayed
		if len(out) == 0 {
			t.Fatal("the caller that stayed got no answer")
		}

		g.mu.Lock()
		defer g.mu.Unlock()

		for i, cancelled := range g.cancelled {
			if cancelled {
				t.Errorf("query %d was cancelled under it", i+1)
			}
		}
	})
}

// Different students do not wait on each other, but no more than the
// slots allow run at once.
func TestEligibilityBatchBoundsConcurrentQueries(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var b eligibilityBatcher

		b.slots = make(chan struct{}, 3)

		g := newGatedLoader()

		const students = 10

		var wg sync.WaitGroup

		for i := range students {
			wg.Go(func() {
				id := "s" + strconv.Itoa(i)

				out, err := b.get(t.Context(), id, g.load)
				if err != nil {
					t.Errorf("get %s: %v", id, err)
				}

				if len(out[id]) != 1 {
					t.Errorf("student %s got someone else's answer", id)
				}
			})
		}

		synctest.Wait()

		if got := g.running.Load(); got != 3 {
			t.Errorf("%d queries running, want the 3 slots full", got)
		}

		close(g.release)
		wg.Wait()

		if got := g.peak.Load(); got > 3 {
			t.Errorf("%d queries ran at once, want at most 3", got)
		}

		if got := g.started.Load(); got != students {
			t.Errorf("%d queries for %d students, want one each", got, students)
		}
	})
}

// A failed query fails every caller that shared it, and the next
// request starts afresh rather than inheriting the failure.
func TestEligibilityBatchErrorIsNotCached(t *testing.T) {
	t.Parallel()

	var b eligibilityBatcher

	fail := func(context.Context, string) (eligibility, error) { return nil, errBoom }

	if _, err := b.get(t.Context(), "s1", fail); !errors.Is(err, errBoom) {
		t.Fatalf("got %v, want the loader's error", err)
	}

	ok := func(context.Context, string) (eligibility, error) { return eligibility{}, nil }

	if _, err := b.get(t.Context(), "s1", ok); err != nil {
		t.Fatalf("the next request inherited %v", err)
	}
}

func TestEligibilitySlots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		pool int32
		want int
	}{{1, 1}, {2, 1}, {16, 8}, {80, 40}} {
		if got := eligibilitySlots(tc.pool); got != tc.want {
			t.Errorf("eligibilitySlots(%d) = %d, want %d", tc.pool, got, tc.want)
		}
	}
}
