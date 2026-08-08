package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Two probes, because they answer two questions with two different
// remedies, and answering them with one endpoint gets the remedy wrong
// in the case that matters.
//
// /healthz is liveness: is this process still a process that can serve?
// Restarting it is the fix when the answer is no.
//
// /readyz is readiness: can it serve *right now*? Taking it out of
// rotation is the fix when the answer is no; restarting it is not,
// because the thing that is wrong is not in this process.
//
// They used to be one endpoint, which read the schema version — so a
// database that was merely slow, or a pool saturated by a full-school
// rush, answered 503, and a supervisor watching that endpoint would
// restart the server at the exact moment it was busiest. The restart
// drops every websocket, empties the counts mirror, and sends twelve
// hundred browsers back to reconnect and refetch — which is more load,
// which saturates the pool again. That is the shape of an outage the
// software causes by being asked the wrong question.

// readyzTimeout bounds the readiness check, so a probe cannot pile up
// behind a database that has stopped answering.
const readyzTimeout = 2 * time.Second

// handleHealthz reports that this process is alive and serving HTTP.
//
// Deliberately touches nothing: no pool, no database, no lock. Its
// answer is "the listener is accepting, the mux is routing, and a
// handler ran" — which is exactly the set of failures a restart
// repairs, and nothing else. If it can answer at all, it answers 200.
//
// Unauthenticated, because a supervisor has no session. Successes are
// not logged, since this is polled forever.
func (app *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok\n")
}

// handleReadyz reports whether this process can serve requests now.
//
// It reads rather than pinging, because a pool that connects to a
// database with no schema in it is not serving anything; and the read
// goes through the pool on purpose, so that a pool with no room left
// is reported as not ready. That is what "not ready" means — send the
// traffic elsewhere — and it is a different instruction from "restart
// this".
//
// What it reads is the version *and* a row from every view, because
// the version alone is a claim the schema makes about itself. Dropping
// one view and leaving schema_version alone produced a process that
// started, stayed up, answered "ready", and served 500s to every
// student — held in rotation indefinitely, with nothing to escalate. A
// half-applied migration is that state.
func (app *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
	defer cancel()

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if _, err := app.queries.ReadyProbe(ctx); err != nil {
		// Warn, not error, for the same reason a 503 from a handler
		// is a warning: a database outage would otherwise page once
		// per probe, sixty times a minute, at the moment the log most
		// needs to be readable.
		app.logWarn(r, logMsgReadyzUnavailable, slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "unavailable\n")

		return
	}

	_, _ = io.WriteString(w, "ready\n")
}
