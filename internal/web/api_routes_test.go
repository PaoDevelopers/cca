package web //nolint:testpackage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Nothing under an API prefix may ever answer with the SPA.
//
// Both areas are served from a subtree pattern that matches every path
// beneath them, so without a catch-all a mistyped route or a wrong
// method is answered with index.html and a 200 — and the client, which
// reads every response as JSON, reports it as an internal error. The
// closed set of error codes has to hold for every request to these
// prefixes, not only the ones that reach a handler.
func TestAPIAreasNeverServeTheSPA(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)
	app.config.Admins = map[string]struct{}{"admin": {}}

	mux := app.router()

	for _, tt := range []struct {
		name   string
		method string
		target string
		want   int
		allow  string
	}{
		{
			name: "a known admin path with the wrong method",
			// Registered only as POST.
			method: http.MethodGet, target: "/admin/api/periods/order",
			want: http.StatusMethodNotAllowed, allow: "POST",
		},
		{
			name:   "a known admin path with several methods",
			method: http.MethodPatch, target: "/admin/api/enrollments",
			want: http.StatusMethodNotAllowed, allow: "DELETE, GET, POST",
		},
		{
			name:   "an admin path that does not exist",
			method: http.MethodGet, target: "/admin/api/nonexistent",
			want: http.StatusNotFound,
		},
		{
			name:   "a deep admin path that does not exist",
			method: http.MethodGet, target: "/admin/api/a/b/c",
			want: http.StatusNotFound,
		},
		{
			name:   "a student path that does not exist",
			method: http.MethodGet, target: "/student/api/nonexistent",
			want: http.StatusNotFound,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(
				t.Context(), tt.method, tt.target, nil))

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}

			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			if strings.Contains(rec.Body.String(), "<!doctype") ||
				strings.Contains(rec.Body.String(), "<html") {
				t.Errorf("an API path answered with HTML: %q", rec.Body.String())
			}

			// RFC 9110 requires Allow on a 405.
			if tt.allow != "" {
				if got := rec.Header().Get("Allow"); got != tt.allow {
					t.Errorf("Allow = %q, want %q", got, tt.allow)
				}
			}
		})
	}
}

// The catch-all must not shadow the real routes: they are more
// specific, so they still win.
func TestTheCatchAllDoesNotShadowRealRoutes(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)

	mux := app.router()

	for _, target := range []string{
		"/admin/api/courses",
		"/admin/api/students",
		"/student/api/courses",
		"/student/api/my_enrollments",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))

		// Unauthenticated, so 401 — but from the route, not the
		// catch-all's 404.
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status = %d, want %d (the route should have matched)",
				target, rec.Code, http.StatusUnauthorized)
		}
	}
}

// The route table is read by every request that misses and written by
// nobody after startup, so nothing about it should need a lock at all
// once it is built. It did: the catch-all sorted the method list in
// place, after the lock was released, on the slice the table itself
// holds. Two simultaneous 405s to the same path then sorted one array
// from two goroutines.
//
// It produced no wrong answer — both sorts agree on the result — which
// is exactly why it survived: only the race detector can see it.
func TestConcurrent405sDoNotRaceOnTheRouteTable(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)

	mux := app.router()

	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			// GET is the one method with no pattern for this path:
			// POST is the reorder, and PUT and DELETE match the
			// sibling {id} route with id="order". So every one of
			// these reaches the catch-all and reads the same method
			// list.
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(
				context.Background(), http.MethodGet, "/admin/api/periods/order", nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}

			if got := rec.Header().Get("Allow"); got != "POST" {
				t.Errorf("Allow = %q, want POST", got)
			}
		})
	}

	wg.Wait()
}

// And a path with several methods, where the sort actually has work to
// do and a torn read would show.
func TestConcurrent405sAgreeOnTheAllowHeader(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)

	mux := app.router()

	seen := make([]string, 64)

	var wg sync.WaitGroup
	for i := range seen {
		wg.Go(func() {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(
				context.Background(), http.MethodPatch, "/admin/api/enrollments", nil))
			seen[i] = rec.Header().Get("Allow")
		})
	}

	wg.Wait()

	for i, got := range seen {
		if got != "DELETE, GET, POST" {
			t.Fatalf("request %d saw Allow = %q", i, got)
		}
	}
}
