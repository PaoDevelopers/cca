package web

import (
	"net/http"
	"slices"
	"strings"
	"sync"
)

// Why the API areas need a catch-all of their own.
//
// Both SPAs are served from a subtree pattern — "/admin/" and
// "/student/" — which matches every path under them, including
// "/admin/api/...". Go's ServeMux synthesizes a 405 only when the
// *sole* matching patterns differ from the request by method; here the
// SPA pattern also matches, so it wins, and a request to an API path
// with the wrong method (or to no API path at all) is answered with
// the SPA's index.html and a 200.
//
// That is worse than a wrong status code. The client's fetch wrapper
// reads the body as JSON, finds HTML, and reports a generic internal
// error — so the closed set of error codes the API documents is
// silently not closed for a whole class of requests, and a mistyped
// route looks like a server fault instead of a 404.
//
// Registering "/admin/api/" and "/student/api/" fixes it: they are
// more specific than the SPA subtree and less specific than every real
// route, so they catch exactly what nothing else claimed. Knowing
// which methods a path does accept then costs nothing, because the
// routes are registered through here anyway — which also lets the 405
// carry the Allow header RFC 9110 requires.
//
// A map keyed by path is not enough for that second part, and it was
// tried: it can only be looked up by the request's literal path, so it
// answered for "/admin/api/courses" and not for
// "/admin/api/courses/ABC". Wildcards are most of the admin API, and
// every one of them answered 404 with no Allow to a wrong method.
//
// Registering each path pattern method-less into the real mux does not
// work either: "/admin/api/students/status" would then match more
// methods than "DELETE /admin/api/students/{id}" while being the more
// specific path, which the mux rejects at registration as the genuine
// ambiguity it is.
//
// So the wildcard matching is done by a second ServeMux that holds
// nothing but the path patterns. Every entry in it accepts every
// method, so no two entries can conflict, and asking it which pattern
// a path belongs to is exactly the question the Allow header needs
// answered — using the same precedence rules as the router itself,
// rather than a second implementation of them.

// apiRoutes remembers which methods each API path accepts, so the
// catch-all can tell "wrong method" from "no such route".
type apiRoutes struct {
	mu      sync.RWMutex
	methods map[string][]string

	// The same path patterns, method-less, for matching a request path
	// to the pattern it belongs to. It never serves a request; only
	// its Handler method is used, for the pattern it returns.
	patterns *http.ServeMux
}

func newAPIRoutes() *apiRoutes {
	//exhaustruct:ignore
	return &apiRoutes{
		methods:  make(map[string][]string),
		patterns: http.NewServeMux(),
	}
}

// record notes one registered pattern. A pattern with no method
// accepts whatever its handler decides, so it is recorded with none
// and the catch-all will never see it: the route itself matches.
func (a *apiRoutes) record(pattern string) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		method, path = "", pattern
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if method == "" {
		a.methods[path] = nil

		return
	}

	if _, seen := a.methods[path]; !seen {
		// Registered once per path, not once per pattern: the mux
		// panics on a duplicate, and GET and POST on one path are two
		// patterns and one path.
		a.patterns.Handle(path, http.NotFoundHandler())
	}

	if !slices.Contains(a.methods[path], method) {
		a.methods[path] = append(a.methods[path], method)
		// Sorted here, once, because the order is a property of the
		// table rather than of any request. Sorting per request meant
		// mutating this slice — the table's own — after the lock was
		// released, so two simultaneous 405s on one path sorted a
		// single array from two goroutines. It produced no wrong
		// answer, which is why only the race detector could see it.
		slices.Sort(a.methods[path])
	}
}

// allowed returns the methods registered for the pattern the request's
// path belongs to, in order, and whether it belongs to one at all.
//
// The result is a copy. Nothing here mutates it today, but handing a
// caller the table's own slice is how the race above happened, and a
// four-element copy on a path that only ever answers 405 is not worth
// reasoning about twice.
func (a *apiRoutes) allowed(r *http.Request) ([]string, bool) {
	_, pattern := a.patterns.Handler(r)
	if pattern == "" {
		return nil, false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	methods, known := a.methods[pattern]

	return slices.Clone(methods), known
}

// apiCatchAll answers everything under an API subtree that no real
// route claimed: 405 with Allow when the path belongs to a pattern
// that exists under other methods, 404 otherwise. Always JSON, never
// the SPA.
func (app *Server) apiCatchAll(routes *apiRoutes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if methods, known := routes.allowed(r); known && len(methods) > 0 {
			w.Header().Set("Allow", strings.Join(methods, ", "))
			app.apiMethodNotAllowed(r, w)

			return
		}

		app.apiError(r, w, http.StatusNotFound, codeNotFound, "no such endpoint", nil)
	}
}
