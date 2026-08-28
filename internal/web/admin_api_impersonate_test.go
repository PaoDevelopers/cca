package web //nolint:testpackage

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func impersonate(t *testing.T, app *Server, id string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/admin/api/students/"+id+"/session", nil)
	r.SetPathValue("id", id)

	rec := httptest.NewRecorder()
	app.apiStudentsImpersonate(rec, r, &UserInfoAdmin{Username: "admin1"})

	return rec
}

func cookieNamed(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}

	return nil
}

// The point of the endpoint: a student session, in the student cookie,
// naming the student that was asked for.
func TestImpersonateMintsAStudentSession(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)

	rec := impersonate(t, app, "s22537")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	cookie := cookieNamed(t, rec, studentCookie)
	if cookie == nil {
		t.Fatalf("no %s cookie was set", studentCookie)
	}

	subject, err := app.sessionKey.decodeSession(roleStudent, cookie.Value, time.Now())
	if err != nil {
		t.Fatalf("decode the minted session: %v", err)
	}

	if subject != "s22537" {
		t.Errorf("subject = %q, want %q", subject, "s22537")
	}
}

// The administrator's own session must survive, or an administrator
// would be signed out of the area they started from every time they
// looked at a student. Nothing here may write the admin cookie — and
// the session it mints is a student one, so it could not carry admin
// rights even if it did.
func TestImpersonateLeavesTheAdminSessionAlone(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.sessionKey = testSessionKey(t)

	if cookie := cookieNamed(t, impersonate(t, app, "s22537"), adminCookie); cookie != nil {
		t.Errorf("the %s cookie was written: %q", adminCookie, cookie.Value)
	}
}

// A cookie naming somebody the roster does not have authenticates to
// an empty catalogue, which looks like a broken student page rather
// than like a typo.
func TestImpersonateRefusesAnUnknownStudent(t *testing.T) {
	t.Parallel()

	app := testServer(pgx.ErrNoRows)
	app.sessionKey = testSessionKey(t)

	rec := impersonate(t, app, "nobody")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	if cookie := cookieNamed(t, rec, studentCookie); cookie != nil {
		t.Errorf("a session was minted anyway: %q", cookie.Value)
	}

	if body := decodeErrorBody(t, rec); body.Error.Code != codeNotFound {
		t.Errorf("code = %q, want %q", body.Error.Code, codeNotFound)
	}
}
