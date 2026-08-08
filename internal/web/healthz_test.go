package web //nolint:testpackage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzReportsAliveWithoutTouchingTheDatabase(t *testing.T) {
	t.Parallel()

	// The distinction that matters: liveness must not depend on the
	// pool. A database that has stopped answering, or a pool saturated
	// by a full-school rush, is not a reason to restart this process —
	// and a restart at that moment drops every websocket and sends
	// twelve hundred browsers back to reconnect, which is more load.
	app := testServer(errTestDatabaseDown)
	w := httptest.NewRecorder()
	app.handleHealthz(w, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d even with the database down", w.Code, http.StatusOK)
	}

	if got := strings.TrimSpace(w.Body.String()); got != "ok" {
		t.Errorf("body = %q, want %q", got, "ok")
	}

	// A probe that is cached is not a probe.
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestReadyzReportsAReachableDatabase(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	w := httptest.NewRecorder()
	app.handleReadyz(w, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if got := strings.TrimSpace(w.Body.String()); got != "ready" {
		t.Errorf("body = %q, want %q", got, "ready")
	}

	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// The point of the readiness endpoint: an unreachable database has to
// make it fail, or a load balancer will keep sending traffic to a
// process that cannot answer.
func TestReadyzFailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	app := testServer(errTestDatabaseDown)
	w := httptest.NewRecorder()
	app.handleReadyz(w, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
