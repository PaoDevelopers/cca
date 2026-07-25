package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~runxiyu/cca/internal/store/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type authFakeDB struct {
	studentID     int64
	studentError  error
	adminError    error
	studentCalls  int
	adminCalls    int
	studentToken  string
	adminToken    string
	adminUsername string
}

func (f *authFakeDB) Exec(_ context.Context, _ string, args ...interface{}) (pgconn.CommandTag, error) {
	f.adminCalls++
	if len(args) == 2 {
		if token, ok := args[0].(pgtype.Text); ok {
			f.adminToken = token.String
		}
		f.adminUsername, _ = args[1].(string)
	}
	if f.adminError != nil {
		return pgconn.CommandTag{}, f.adminError
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (f *authFakeDB) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query call")
}

func (f *authFakeDB) QueryRow(_ context.Context, _ string, args ...interface{}) pgx.Row {
	f.studentCalls++
	if len(args) == 2 {
		if token, ok := args[0].(pgtype.Text); ok {
			f.studentToken = token.String
		}
		if studentID, ok := args[1].(int64); ok {
			f.studentID = studentID
		}
	}
	return authFakeRow{id: f.studentID, err: f.studentError}
}

type authFakeRow struct {
	id  int64
	err error
}

func (r authFakeRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected Scan destination count")
	}
	id, ok := dest[0].(*int64)
	if !ok {
		return errors.New("unexpected Scan destination type")
	}
	*id = r.id
	return nil
}

func newTestAuthApp(fake *authFakeDB) *App {
	app := &App{
		queries: db.New(fake),
	}
	app.config.TestAuth.Enabled = true
	app.config.Admins = map[string]struct{}{"henry": {}}
	return app
}

func newLocalTestAuthRequest(method, body string) *http.Request {
	r := httptest.NewRequest(method, "http://localhost/api/v1/test-auth", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:43210"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost")
	return r
}

func TestAPITestAuthDisabled(t *testing.T) {
	app := &App{}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			r := newLocalTestAuthRequest(method, `{}`)
			w := httptest.NewRecorder()
			app.handleAPITestAuth(w, r)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", w.Code)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("disabled test authentication emitted a cookie")
			}
		})
	}
}

func TestTestAuthModeDisablesOIDCAndLegacyBypass(t *testing.T) {
	app := &App{}
	app.config.TestAuth.Enabled = true
	r := httptest.NewRequest(http.MethodPost, "http://localhost/auth", strings.NewReader("bypass=12345"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	app.handleAuth(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("disabled legacy bypass emitted a cookie")
	}
}

func TestIndexRedirectsLocalTestModeToLogin(t *testing.T) {
	app := &App{}
	app.config.TestAuth.Enabled = true
	r := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	app.handleIndex(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/test-login" {
		t.Fatalf("index response = %d %q, want 303 /test-login", w.Code, w.Header().Get("Location"))
	}
}

func TestAPITestAuthMetadataAndMethodSafety(t *testing.T) {
	fake := &authFakeDB{}
	app := newTestAuthApp(fake)

	get := newLocalTestAuthRequest(http.MethodGet, "")
	getRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRecorder.Code)
	}
	if fake.studentCalls != 0 || fake.adminCalls != 0 || len(getRecorder.Result().Cookies()) != 0 {
		t.Fatal("GET test-auth metadata had authentication side effects")
	}

	put := newLocalTestAuthRequest(http.MethodPut, `{}`)
	putRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(putRecorder, put)
	if putRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status = %d, want 405", putRecorder.Code)
	}
	if got := putRecorder.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow = %q, want GET, POST", got)
	}
	if fake.studentCalls != 0 || fake.adminCalls != 0 || len(putRecorder.Result().Cookies()) != 0 {
		t.Fatal("unsupported method had authentication side effects")
	}
}

func TestAPITestAuthStudentLogin(t *testing.T) {
	fake := &authFakeDB{studentID: 12345}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":" S12345 "}`)
	w := httptest.NewRecorder()
	before := time.Now()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if fake.studentCalls != 1 || fake.studentID != 12345 || fake.studentToken == "" {
		t.Fatalf("student DB call = (%d, %d, %q), want one persisted token for 12345", fake.studentCalls, fake.studentID, fake.studentToken)
	}
	assertSessionCookie(t, w.Result().Cookies(), "student:"+fake.studentToken, false, before)
	var response testAuthLoginResult
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RedirectTo != "/student/" {
		t.Fatalf("redirect_to = %q, want /student/", response.RedirectTo)
	}
	if strings.Contains(w.Body.String(), fake.studentToken) {
		t.Fatal("response leaked the session token")
	}
}

func TestAPITestAuthAdminLogin(t *testing.T) {
	fake := &authFakeDB{}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"admin","identifier":"henry"}`)
	w := httptest.NewRecorder()
	before := time.Now()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if fake.adminCalls != 1 || fake.adminUsername != "henry" || fake.adminToken == "" {
		t.Fatalf("admin DB call = (%d, %q, %q), want configured admin token", fake.adminCalls, fake.adminUsername, fake.adminToken)
	}
	assertSessionCookie(t, w.Result().Cookies(), "admin:"+fake.adminToken, false, before)
	var response testAuthLoginResult
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RedirectTo != "/admin/" {
		t.Fatalf("redirect_to = %q, want /admin/", response.RedirectTo)
	}
}

func TestAPITestAuthRejectsUnknownAdminBeforeUpsert(t *testing.T) {
	fake := &authFakeDB{}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"admin","identifier":"not-configured"}`)
	w := httptest.NewRecorder()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if fake.adminCalls != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatal("unknown admin reached the upsert or received a cookie")
	}
}

func TestAPITestAuthRejectsCrossSiteBeforeDatabase(t *testing.T) {
	fake := &authFakeDB{studentID: 12345}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":"12345"}`)
	r.Header.Set("Origin", "http://attacker.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if fake.studentCalls != 0 || len(w.Result().Cookies()) != 0 {
		t.Fatal("cross-site login reached the database or received a cookie")
	}
}

func TestAPITestAuthDatabaseFailureDoesNotEmitCookie(t *testing.T) {
	fake := &authFakeDB{studentID: 12345, studentError: errors.New("database unavailable")}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":"12345"}`)
	w := httptest.NewRecorder()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("database failure emitted a session cookie")
	}
}

func TestAPITestAuthAdminDatabaseFailureDoesNotEmitCookie(t *testing.T) {
	fake := &authFakeDB{adminError: errors.New("database unavailable")}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"admin","identifier":"henry"}`)
	w := httptest.NewRecorder()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("admin database failure emitted a session cookie")
	}
}

func TestAPITestAuthMissingStudentDoesNotEmitCookie(t *testing.T) {
	fake := &authFakeDB{studentError: pgx.ErrNoRows}
	app := newTestAuthApp(fake)
	r := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":"12345"}`)
	w := httptest.NewRecorder()
	app.handleAPITestAuth(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("missing student received a session cookie")
	}
}

func TestAPITestAuthRemoteModeRequiresAccessKey(t *testing.T) {
	fake := &authFakeDB{studentID: 12345}
	app := newTestAuthApp(fake)
	app.config.TestAuth.AllowRemote = true
	app.config.TestAuth.AccessKey = "development-key-1234"

	badRequest := httptest.NewRequest(http.MethodPost, "https://test.example/api/v1/test-auth", strings.NewReader(`{"role":"student","identifier":"12345","access_key":"wrong"}`))
	badRequest.Header.Set("Content-Type", "application/json")
	badRequest.Header.Set("Origin", "https://test.example")
	badRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(badRecorder, badRequest)
	if badRecorder.Code != http.StatusForbidden || fake.studentCalls != 0 {
		t.Fatalf("bad access key status/calls = %d/%d, want 403/0", badRecorder.Code, fake.studentCalls)
	}

	goodRequest := httptest.NewRequest(http.MethodPost, "https://test.example/api/v1/test-auth", strings.NewReader(`{"role":"student","identifier":"12345","access_key":"development-key-1234"}`))
	goodRequest.Header.Set("Content-Type", "application/json")
	goodRequest.Header.Set("Origin", "https://test.example")
	goodRequest.TLS = &tls.ConnectionState{}
	goodRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(goodRecorder, goodRequest)
	if goodRecorder.Code != http.StatusOK {
		t.Fatalf("good access key status = %d, want 200; body=%s", goodRecorder.Code, goodRecorder.Body.String())
	}
	assertSessionCookie(t, goodRecorder.Result().Cookies(), "student:"+fake.studentToken, true, time.Now().Add(-time.Second))
}

func TestAPITestAuthConfiguredAccessKeyIsRequiredLocally(t *testing.T) {
	fake := &authFakeDB{studentID: 12345}
	app := newTestAuthApp(fake)
	app.config.TestAuth.AccessKey = "local-test-key"

	metadataRequest := newLocalTestAuthRequest(http.MethodGet, "")
	metadataRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(metadataRecorder, metadataRequest)
	var metadata testAuthView
	if err := json.Unmarshal(metadataRecorder.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode test-auth metadata: %v", err)
	}
	if !metadata.RequiresAccessKey {
		t.Fatal("configured local access key was not advertised to the login page")
	}

	badRequest := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":"12345","access_key":"wrong"}`)
	badRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(badRecorder, badRequest)
	if badRecorder.Code != http.StatusForbidden || fake.studentCalls != 0 {
		t.Fatalf("bad local access key status/calls = %d/%d, want 403/0", badRecorder.Code, fake.studentCalls)
	}

	goodRequest := newLocalTestAuthRequest(http.MethodPost, `{"role":"student","identifier":"12345","access_key":"local-test-key"}`)
	goodRecorder := httptest.NewRecorder()
	app.handleAPITestAuth(goodRecorder, goodRequest)
	if goodRecorder.Code != http.StatusOK || fake.studentCalls != 1 {
		t.Fatalf("good local access key status/calls = %d/%d, want 200/1; body=%s", goodRecorder.Code, fake.studentCalls, goodRecorder.Body.String())
	}
}

func TestTestAuthLocalRestriction(t *testing.T) {
	app := &App{}
	app.config.TestAuth.Enabled = true

	local := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/test-auth", nil)
	local.RemoteAddr = "127.0.0.1:1234"
	if !app.testAuthRequestAllowed(local) {
		t.Fatal("loopback request was rejected")
	}

	forwarded := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/test-auth", nil)
	forwarded.RemoteAddr = "127.0.0.1:1234"
	forwarded.Header.Set("X-Forwarded-For", "203.0.113.10")
	if app.testAuthRequestAllowed(forwarded) {
		t.Fatal("forwarded remote request was accepted as local")
	}

	publicHost := httptest.NewRequest(http.MethodGet, "http://test.example/api/v1/test-auth", nil)
	publicHost.RemoteAddr = "127.0.0.1:1234"
	if app.testAuthRequestAllowed(publicHost) {
		t.Fatal("public-host request was accepted as local")
	}
}

func TestParseTestStudentID(t *testing.T) {
	for _, input := range []string{"12345", "s12345", " S12345 "} {
		got, err := parseTestStudentID(input)
		if err != nil || got != 12345 {
			t.Errorf("parseTestStudentID(%q) = (%d, %v), want (12345, nil)", input, got, err)
		}
	}
	for _, input := range []string{"", "s", "ss12345", "0", "-1", "not-a-number", "9223372036854775808"} {
		if got, err := parseTestStudentID(input); err == nil {
			t.Errorf("parseTestStudentID(%q) = %d, want error", input, got)
		}
	}
}

func assertSessionCookie(t *testing.T, cookies []*http.Cookie, wantValue string, wantSecure bool, before time.Time) {
	t.Helper()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "session" || cookie.Value != wantValue {
		t.Fatalf("cookie = %s=%q, want session=%q", cookie.Name, cookie.Value, wantValue)
	}
	if cookie.Path != "/" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Secure != wantSecure {
		t.Fatalf("cookie flags = Path:%q HttpOnly:%v SameSite:%v Secure:%v", cookie.Path, cookie.HttpOnly, cookie.SameSite, cookie.Secure)
	}
	wantExpiry := before.Add(72 * time.Hour)
	if cookie.Expires.Before(wantExpiry.Add(-time.Minute)) || cookie.Expires.After(wantExpiry.Add(time.Minute)) {
		t.Fatalf("cookie expiry = %s, want around %s", cookie.Expires, wantExpiry)
	}
}
