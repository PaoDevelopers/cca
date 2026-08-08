package web

import (
	"context"
	"time"
)

// writeTimeout bounds a single database write. Generous, because a
// write may queue behind another administrator's lock; short enough
// that a stuck one cannot hold a pool connection all day.
const writeTimeout = 30 * time.Second

// readTimeout bounds a single database read.
//
// Reads keep the request's cancellation, because abandoning work
// nobody is waiting for is exactly right — but cancellation only
// arrives when the client's connection breaks, and a client whose
// machine went to sleep, or whose NAT dropped the mapping, breaks
// nothing observable for minutes. Until then the query holds a pool
// connection, and enough of them holds every pool connection, at
// which point the process is down for everyone including /healthz.
//
// Generous against the slowest read in the system: eligibility asks
// the database the whole rule set once per course, and under a
// full-school rush its 99th percentile is a few seconds.
const readTimeout = 30 * time.Second

// readCtx is the context a read runs under: the request's, with a
// ceiling. See readTimeout.
func readCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, readTimeout)
}

// writeCtx is the context a write runs under: derived from the
// request, detached from its cancellation, bounded by its own timeout.
//
// The reason is what happens after the commit. A handler writes, and
// then tells the hub what changed — the invalidation frames and the
// course counts every other browser is waiting for. If the request's
// context is cancelled midway, because the student navigated away or
// closed the tab, the driver sends a cancel whose outcome is genuinely
// indeterminate: the statement may already have committed. The handler
// then takes its error path and returns without ever reaching the
// broadcast, so the row exists and nobody is told. The hub's count
// mirror does not self-heal, because only a course that was written is
// published — and this course was written by a request that reported
// failure.
//
// Detaching costs the ability to abandon work a client no longer wants
// — which for a write is not a thing worth having: the work is either
// done or not, and finding out is cheaper than guessing. The timeout
// is what bounds it instead.
//
// Values are kept, so the request's logging attributes travel with it.
func writeCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), writeTimeout)
}
