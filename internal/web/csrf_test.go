package web //nolint:testpackage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The wrapped handler records whether the request got through, so a
// test can tell "allowed" from "answered by the deny handler" without
// depending on what the real handlers would have done.
func protectedTestHandler(t *testing.T) (http.Handler, *bool) {
	t.Helper()

	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true

		w.WriteHeader(http.StatusNoContent)
	})

	app := testServer(nil)

	return app.crossOriginProtection().Handler(inner), &reached
}

func TestCrossOriginProtection(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		method  string
		target  string
		fetch   string // Sec-Fetch-Site
		origin  string
		allowed bool
	}{
		{
			name:    "same-origin write",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			fetch:   "same-origin",
			allowed: true,
		},
		{
			name:    "cross-site write",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			fetch:   "cross-site",
			allowed: false,
		},
		{
			// The reason this exists at all: the session cookies are
			// SameSite=Lax, so a sibling host under the same
			// registrable domain still sends them. Lax does not stop
			// this request; same-origin does.
			name:    "same-site but cross-origin write",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			fetch:   "same-site",
			allowed: false,
		},
		{
			// Multipart is sent cross-origin without a preflight, so
			// the CSV imports are the most exposed writes.
			name:    "cross-site CSV import",
			method:  http.MethodPost,
			target:  "/admin/api/students/import",
			fetch:   "cross-site",
			allowed: false,
		},
		{
			name:    "cross-site logout",
			method:  http.MethodPost,
			target:  "/student/logout",
			fetch:   "cross-site",
			allowed: false,
		},
		{
			name:    "cross-site read",
			method:  http.MethodGet,
			target:  "/admin/api/groups",
			fetch:   "cross-site",
			allowed: true,
		},
		{
			// The event websockets upgrade over GET.
			name:    "cross-site websocket upgrade",
			method:  http.MethodGet,
			target:  "/student/api/events",
			fetch:   "cross-site",
			allowed: true,
		},
		{
			// The OIDC callback is a cross-site form_post by design.
			name:    "the OIDC callback",
			method:  http.MethodPost,
			target:  "/auth",
			fetch:   "cross-site",
			allowed: true,
		},
		{
			// Not a browser, or a browser too old for Sec-Fetch-Site
			// and sending no Origin: allowed, as the standard library
			// documents.
			name:    "no browser headers at all",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			allowed: true,
		},
		{
			name:    "old browser, Origin matching Host",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			origin:  "https://cca.example.org",
			allowed: true,
		},
		{
			name:    "old browser, Origin not matching Host",
			method:  http.MethodPost,
			target:  "/admin/api/groups",
			origin:  "https://evil.example.com",
			allowed: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, reached := protectedTestHandler(t)

			r := httptest.NewRequestWithContext(
				t.Context(), tt.method, "https://cca.example.org"+tt.target, nil,
			)
			if tt.fetch != "" {
				r.Header.Set("Sec-Fetch-Site", tt.fetch)
			}

			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if *reached != tt.allowed {
				t.Fatalf("request reached the handler = %v, want %v (status %d)",
					*reached, tt.allowed, w.Code)
			}

			if tt.allowed {
				return
			}

			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
			}

			// Rejections use the same envelope as every other refusal,
			// so the frontend's error handling needs no special case.
			body := decodeErrorBody(t, w)
			if body.Error.Code != codeForbidden {
				t.Errorf("code = %q, want %q", body.Error.Code, codeForbidden)
			}

			if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
				t.Errorf("Content-Type = %q, want JSON", w.Header().Get("Content-Type"))
			}
		})
	}
}

// The bypass is registered as a ServeMux pattern, which is method
// scoped: a cross-site GET of /auth is safe anyway, but a cross-site
// PUT must not inherit the exemption.
func TestOIDCBypassIsNarrow(t *testing.T) {
	t.Parallel()

	handler, reached := protectedTestHandler(t)

	r := httptest.NewRequestWithContext(
		t.Context(), http.MethodPut, "https://cca.example.org/auth", nil,
	)
	r.Header.Set("Sec-Fetch-Site", "cross-site")

	handler.ServeHTTP(httptest.NewRecorder(), r)

	if *reached {
		t.Error("a cross-site PUT to /auth was allowed; the bypass should cover POST only")
	}
}
