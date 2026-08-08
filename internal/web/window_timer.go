package web

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// The enrollment window is defined by two timestamps per grade, and
// openness is derived from them wherever it is read: by v_grades, by
// the write functions' gates, and by the frontends through the grade
// document. Nothing is stored about whether a window "has opened", so
// nothing has to be repaired after downtime, a clock adjustment, or a
// missed tick. A student acting one second after closes_at is refused
// by the database whether or not anyone was told.
//
// That leaves exactly one job for a timer: repainting a browser that
// is already looking at the page when a bound passes. Missing it costs
// a stale button until the next refetch or reconnect, which is why
// this is deliberately the least reliable component in the system and
// is allowed to be.
//
// One timer, not one per grade: it is armed at the earliest bound
// still in the future across every grade, and re-armed after it fires
// and after any write that could move a bound.

// windowTimerRetry is how long to wait before asking for the next
// boundary again after the read failed.
const windowTimerRetry = time.Minute

// windowTimerSkewBackoff is how long to wait when the database still
// calls a boundary future that we have already fired for. See rearm.
const windowTimerSkewBackoff = time.Second

// windowTimer holds the single armed timer. The zero value is not
// usable; newWindowTimer makes one.
type windowTimer struct {
	mu    sync.Mutex
	timer *time.Timer

	// The boundary the armed timer last fired for. Only ever compared
	// against a later reading of the same query, to notice that we have
	// come back round to a boundary the database has not passed yet.
	fired time.Time

	// Closed when the server shuts down, so a firing timer does not
	// outlive the hub it broadcasts to.
	done <-chan struct{}
}

// rearmWindowTimer recomputes the next boundary and re-arms. Called
// at startup, after the timer fires, and after any write that can move
// a bound.
//
// The caller's context is detached deliberately: a handler calls this
// after its write has committed and then returns, cancelling its
// context, and the boundary read must not be cancelled with it. Values
// (the request's logging attributes) are carried through; the
// cancellation is not.
//
// Failure is logged and dropped rather than propagated, for the same
// reason: the write has already committed, and a timer that did not
// arm costs a stale button, not a wrong answer.
func (app *Server) rearmWindowTimer(parent context.Context) {
	if app.windowTimer == nil {
		return
	}

	app.windowTimer.rearm(context.WithoutCancel(parent), app)
}

func newWindowTimer(done <-chan struct{}) *windowTimer {
	//exhaustruct:ignore
	return &windowTimer{done: done}
}

func (wt *windowTimer) rearm(parent context.Context, app *Server) {
	// A short timeout of its own: this runs on a request path, and a
	// slow database must not hold up the response that already
	// succeeded.
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	next, err := app.queries.NextWindowBoundary(ctx)
	if err != nil {
		slog.Warn(logMsgWindowTimerError, slog.Any("error", err))

		// Ask again later rather than give up. Simply returning left
		// the spent timer in the field with nothing to replace it, so
		// one hiccup at one boundary silenced every boundary after it
		// until an administrator happened to write a window or the
		// process restarted. "Unreliable" here is allowed to mean
		// occasionally late; it is not allowed to mean fires once and
		// then stops.
		wt.arm(windowTimerRetry, time.Time{}, parent, app)

		return
	}

	if !next.Valid {
		// Every bound is behind us; nothing to wait for until a
		// window is written again.
		wt.disarm()

		return
	}

	// A bound that has just passed while we were reading gives a
	// non-positive delay, which time.AfterFunc fires immediately —
	// the correct behaviour, since the boundary has indeed passed.
	delay := time.Until(next.Time)

	// Unless it has not. next.Time is the database's reading and
	// time.Until is ours, and the query only returns bounds the
	// database still calls future. So a non-positive delay for the
	// boundary we just fired for means our clock is ahead of the
	// database's, not that anything passed: firing again would
	// broadcast, re-arm, get the same answer, and spin — one round
	// trip and one broadcast to every open page per iteration, for as
	// long as the skew lasts. Waiting lets the database catch up.
	if delay <= 0 && next.Time.Equal(wt.lastFired()) {
		slog.Warn(logMsgWindowTimerSkew,
			slog.Time("boundary", next.Time),
			slog.Duration("behind", -delay))

		delay = windowTimerSkewBackoff
	}

	slog.Info(logMsgWindowTimerArmed,
		slog.Time("boundary", next.Time),
		slog.Duration("delay", delay))

	wt.arm(delay, next.Time, parent, app)
}

func (wt *windowTimer) lastFired() time.Time {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	return wt.fired
}

func (wt *windowTimer) disarm() {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	if wt.timer != nil {
		wt.timer.Stop()
		wt.timer = nil
	}
}

// arm replaces whatever is armed with a timer for delay.
//
// boundary is the instant being waited for, or the zero time when this
// is only a retry after a failed read — the difference being that a
// retry has no boundary to announce, so it re-reads without
// broadcasting.
func (wt *windowTimer) arm(
	delay time.Duration, boundary time.Time,
	parent context.Context, app *Server,
) {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	if wt.timer != nil {
		wt.timer.Stop()
	}

	wt.timer = time.AfterFunc(delay, func() {
		select {
		case <-wt.done:
			return
		default:
		}

		if !boundary.IsZero() {
			wt.mu.Lock()
			wt.fired = boundary
			wt.mu.Unlock()

			slog.Info(logMsgWindowTimerFired)
			// Every open page re-reads the grade document, and with
			// it is_open. The refetch is the repair: the broadcast
			// carries no state of its own, so a client that misses it
			// and one that receives it converge on the same answer.
			app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		}

		// Bounds are not consumed by passing, so the next one may
		// well be another grade's, or this grade's closes_at.
		wt.rearm(context.WithoutCancel(parent), app)
	})
}
