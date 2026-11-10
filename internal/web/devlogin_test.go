package web //nolint:testpackage

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// -dev-login is the whole of authentication when it is on, so a
// listener anyone else can reach must keep the process from starting.
func TestDevLoginOnlyListensOnLoopback(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		network, address string
		ok               bool
	}{
		{"tcp", "127.0.0.1:8080", true},
		{"tcp", "[::1]:8080", true},
		{"tcp", "localhost:8080", true},
		{"tcp", ":8080", false},
		{"tcp", "0.0.0.0:8080", false},
		{"tcp", "10.0.0.5:8080", false},
		{"tcp", "cca.example.org:443", false},
		{"unix", "/run/cca.sock", false},
	} {
		err := checkLoopback(tt.network, tt.address)
		if (err == nil) != tt.ok {
			t.Errorf("checkLoopback(%q, %q) = %v, want ok=%v", tt.network, tt.address, err, tt.ok)
		}

		if err != nil && !errors.Is(err, errDevLoginExposed) {
			t.Errorf("checkLoopback(%q, %q) = %v, want errDevLoginExposed", tt.network, tt.address, err)
		}
	}
}

// The administrator list still applies: dev login skips proving who you
// are, not what that account may do.
func TestDevLoginGoesThroughTheAdminList(t *testing.T) {
	t.Parallel()

	app := &Server{devLogin: true, sessionKey: testSessionKey(t)}
	app.config.Admins = map[string]struct{}{"dev.admin": {}}

	// The recorder rather than its Result: status, headers and cookies
	// are all this reads, and a Result carries a body to close.
	post := func(form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1/dev/login",
			strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		w := httptest.NewRecorder()
		app.handleDevLogin(w, r)

		return w
	}

	cookie := func(res *httptest.ResponseRecorder, name string) string {
		for _, line := range res.Header().Values("Set-Cookie") {
			if c, err := http.ParseSetCookie(line); err == nil && c.Name == name {
				return c.Value
			}
		}

		return ""
	}

	res := post(url.Values{"role": {roleAdmin}, "email": {"dev.admin"}})
	if res.Code != http.StatusSeeOther || res.Header().Get("Location") != "/admin/" {
		t.Fatalf("listed admin: status %d to %q, want 303 to /admin/", res.Code, res.Header().Get("Location"))
	}

	subject, err := app.sessionKey.decodeSession(roleAdmin, cookie(res, adminCookie), time.Now())
	if err != nil || subject != "dev.admin" {
		t.Errorf("listed admin: session subject %q, err %v", subject, err)
	}

	res = post(url.Values{"role": {roleAdmin}, "email": {"someone.else"}})
	if cookie(res, adminCookie) != "" || cookie(res, flashCookie) != "not_admin" {
		t.Errorf("unlisted admin was not refused: cookies %v", res.Header().Values("Set-Cookie"))
	}

	if res = post(url.Values{"role": {"root"}, "email": {"dev.admin"}}); res.Code != http.StatusBadRequest {
		t.Errorf("unknown role: status %d, want 400", res.Code)
	}
}
