package web

import (
	"context"
	"fmt"
	"sync"
)

// Why one student's eligibility reads share a query.
//
// Eligibility is the most expensive read in the system, and one student
// is rarely one page: a laptop, a phone, a tab left open from the
// morning, and every one of them re-asks at the same moments — after
// the student's own write, after a broadcast, after a reconnect. In a
// live window with about four hundred students that was several
// eligibility queries per student per event, all identical, and
// together they held the database at full load.
//
// So requests for one student are batched: at most one query runs for
// a student at a time, and every request that arrives while it runs
// waits for the single query that follows it, which all of them share.
//
// Joining the running query instead would be wrong. It may have read
// the database before the write that prompted the new request — the
// student's own enrollment in another tab, say — and answering with it
// would show a verdict from before a change the student has just seen
// succeed, with nothing left to trigger a correction. Every answer here
// comes from a query that started after the request arrived, which is
// exactly what an unbatched read promised.
//
// The shared query does not run under any one request's context: one
// tab closing must not fail the others, and a cancelled query costs
// more than a finished one (see openDatabase). It keeps the first
// request's values, for logging, and gets its own ceiling.
//
// slots bounds how many eligibility queries run at once across all
// students, below the pool size, so that a burst of reads cannot take
// every connection and leave the writes — the thing the students are
// actually waiting on — queueing behind them.

// eligibilityLoader runs the query for one student.
type eligibilityLoader func(ctx context.Context, studentID string) (eligibility, error)

type eligibilityBatcher struct {
	mu      sync.Mutex
	flights map[string]*eligibilityFlights

	// nil means unbounded; the zero value is usable, for tests that
	// build a bare Server.
	slots chan struct{}
}

// The query running for one student, and the one queued behind it.
type eligibilityFlights struct {
	running *eligibilityCall
	queued  *eligibilityCall
}

type eligibilityCall struct {
	done chan struct{}
	out  eligibility
	err  error
}

// eligibilitySlots is how many eligibility queries may run at once for
// a pool of maxConns connections: half of it, leaving the other half
// for writes and the cheap reads.
func eligibilitySlots(maxConns int32) int {
	return max(1, int(maxConns)/2)
}

// get answers for one student from a query that started after this
// call did, sharing it with every other caller for the same student
// that is waiting at the same time. ctx bounds only the wait.
func (b *eligibilityBatcher) get(ctx context.Context, studentID string, load eligibilityLoader) (eligibility, error) {
	b.mu.Lock()

	if b.flights == nil {
		b.flights = make(map[string]*eligibilityFlights)
	}

	var call *eligibilityCall

	switch flights := b.flights[studentID]; {
	case flights == nil:
		call = &eligibilityCall{done: make(chan struct{}), out: nil, err: nil}
		b.flights[studentID] = &eligibilityFlights{running: call, queued: nil}

		go b.execute(context.WithoutCancel(ctx), studentID, call, load)
	case flights.queued == nil:
		call = &eligibilityCall{done: make(chan struct{}), out: nil, err: nil}
		flights.queued = call
	default:
		call = flights.queued
	}

	b.mu.Unlock()

	select {
	case <-call.done:
		return call.out, call.err
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for eligibility: %w", ctx.Err())
	}
}

// execute runs a student's queries back to back until none is queued.
// The queued call is only promoted once the running one has finished,
// so it starts after every request that joined it arrived.
func (b *eligibilityBatcher) execute(ctx context.Context, studentID string, call *eligibilityCall, load eligibilityLoader) {
	for {
		call.out, call.err = b.run(ctx, studentID, load)
		close(call.done)

		b.mu.Lock()

		flights := b.flights[studentID]
		next := flights.queued

		if next == nil {
			delete(b.flights, studentID)
			b.mu.Unlock()

			return
		}

		flights.running, flights.queued = next, nil
		b.mu.Unlock()

		call = next
	}
}

func (b *eligibilityBatcher) run(ctx context.Context, studentID string, load eligibilityLoader) (eligibility, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if b.slots != nil {
		select {
		case b.slots <- struct{}{}:
			defer func() { <-b.slots }()
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for an eligibility slot: %w", ctx.Err())
		}
	}

	return load(ctx, studentID)
}
