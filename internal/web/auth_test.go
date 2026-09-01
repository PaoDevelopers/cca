package web //nolint:testpackage

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The nonce cookie must be SameSite=None: the OIDC callback arrives as
// a cross-site form_post, which SameSite=Lax cookies are not sent on.
// This pins that requirement so it cannot be "tidied" back to Lax.
func TestSigninNonceCookieSurvivesFormPost(t *testing.T) {
	t.Parallel()

	app := &Server{}
	app.config.OIDC.Authorize = "https://login.example.com/authorize"
	app.config.OIDC.Client = "client-id"

	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://cca.example.org/student/", nil)
	app.serveSignin(w, r, roleStudent, "")

	var nonce *http.Cookie

	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == nonceCookie {
			nonce = cookie
		}
	}

	if nonce == nil {
		t.Fatalf("sign-in page did not set the %s cookie", nonceCookie)
	}

	if nonce.SameSite != http.SameSiteNoneMode {
		t.Errorf("nonce cookie SameSite = %v, want None (Lax cookies are not sent on the cross-site form_post back to /auth)", nonce.SameSite)
	}

	if !nonce.Secure || !nonce.HttpOnly {
		t.Errorf("nonce cookie must be Secure and HttpOnly, got Secure=%v HttpOnly=%v", nonce.Secure, nonce.HttpOnly)
	}
}

var errTestDatabaseDown = errors.New("connection refused")

// A cookie that is not a live session for this area is a 401, however
// it fails: forged, tampered, expired, or signed for the other area.
// They are one answer because a stateless session cannot fail to be
// checked — there is no case where "try again shortly" is right.
func TestOnlyAWellSignedLiveCookieAuthenticates(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)

	valid, err := key.encodeSession(roleStudent, "s42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	adminCookieValue, err := key.encodeSession(roleAdmin, "test.admin", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode admin: %v", err)
	}

	expired, err := key.encodeSession(roleStudent, "s42", time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("encode expired: %v", err)
	}

	otherKey, err := newSessionKey(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatalf("other key: %v", err)
	}

	foreign, err := otherKey.encodeSession(roleStudent, "s42", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode foreign: %v", err)
	}

	for _, tt := range []struct {
		name  string
		value string
		want  int
	}{
		{"a live student session", valid, http.StatusOK},
		{"no cookie at all", "", http.StatusUnauthorized},
		{"not a session", "a-real-looking-token", http.StatusUnauthorized},
		{"expired", expired, http.StatusUnauthorized},
		{"signed by another key", foreign, http.StatusUnauthorized},
		{"signed for the admin area", adminCookieValue, http.StatusUnauthorized},
		{"payload edited under the signature", strings.Replace(valid, "s42", "s43", 1), http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			app.sessionKey = key

			handler := app.studentAPI("handleStuAPIPeriods", app.handleStuAPIPeriods)

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/student/api/periods", nil)
			if tt.value != "" {
				//exhaustruct:ignore // the request side of a cookie is name and value
				r.AddCookie(&http.Cookie{
					Name: studentCookie, Value: tt.value,
					Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
				})
			}

			w := httptest.NewRecorder()
			handler(w, r)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tt.want, w.Body.String())
			}

			if tt.want == http.StatusUnauthorized {
				if body := decodeErrorBody(t, w); body.Error.Code != codeUnauthenticated {
					t.Errorf("code = %q, want %q", body.Error.Code, codeUnauthenticated)
				}
			}
		})
	}
}

// The allowlist is configuration, not part of the cookie, so removing
// somebody from it takes their access away even while their cookie is
// still well signed and unexpired.
func TestAdminAllowlistIsRecheckedPerRequest(t *testing.T) {
	t.Parallel()

	key := testSessionKey(t)

	token, err := key.encodeSession(roleAdmin, "test.admin", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	request := func() *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin/api/periods", nil)
		//exhaustruct:ignore // the request side of a cookie is name and value
		r.AddCookie(&http.Cookie{
			Name: adminCookie, Value: token,
			Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})

		return r
	}

	app := testServer(nil)
	app.sessionKey = key
	app.config.Admins = map[string]struct{}{"test.admin": {}}

	handler := app.adminAPI("apiPeriodsList", app.apiPeriodsList)

	w := httptest.NewRecorder()
	handler(w, request())

	if w.Code != http.StatusOK {
		t.Fatalf("an allowlisted administrator got %d (body %q)", w.Code, w.Body.String())
	}

	// Same cookie, same key, same expiry — only the configuration
	// changed.
	app.config.Admins = map[string]struct{}{}

	w = httptest.NewRecorder()
	handler(w, request())

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a de-listed administrator got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// Every bounded cookie must express its lifetime as Max-Age.
//
// Expires is compared against the browser's clock, not against the
// response's Date header, so a device running fast throws our cookies
// away as it receives them. For the nonce that is unrecoverable: the
// callback finds no cookie, sign-in fails, and trying again fails the
// same way. Max-Age is a countdown from receipt, so the two clocks
// never have to agree.
func TestBoundedCookiesCountDownRatherThanNameAnInstant(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	setCookie(rec, studentCookie, "value", 72*time.Hour)
	setCookie(rec, flashCookie, "code", time.Minute)

	app := &Server{}
	app.config.OIDC.Authorize = "https://login.example.com/authorize"
	app.config.OIDC.Client = "client-id"
	app.serveSignin(rec, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/", nil), "student", "")

	cookies := rec.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("set %d cookies, want 3", len(cookies))
	}

	for _, c := range cookies {
		if c.MaxAge <= 0 {
			t.Errorf("%s: MaxAge = %d, want a positive countdown",
				c.Name, c.MaxAge)
		}

		if !c.Expires.IsZero() {
			t.Errorf("%s: names the instant %v, which the browser "+
				"judges against its own clock", c.Name, c.Expires)
		}
	}
}
