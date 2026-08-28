package web //nolint:testpackage

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PaoDevelopers/cca/internal/db"
)

// Discard the handlers' logging.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

type fakeDBTX struct {
	err error
}

func (f fakeDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}

func (f fakeDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if f.err != nil {
		return nil, f.err
	}

	return emptyRows{}, nil
}

// A result set with no rows, so handlers that read a list after writing
// run to completion instead of dereferencing a nil Rows.
type emptyRows struct{}

func (emptyRows) Close()                                       {}
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Scan(...any) error                            { return nil }
func (emptyRows) Values() ([]any, error)                       { return nil, nil }
func (emptyRows) RawValues() [][]byte                          { return nil }
func (emptyRows) Conn() *pgx.Conn                              { return nil }

func (f fakeDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return errRow{scanErr: f.err}
}

type errRow struct {
	scanErr error
}

func (r errRow) Scan(...any) error { return r.scanErr }

// Every write fails with err; nil for a server whose writes succeed.
func testServer(err error) *Server {
	queries := db.New(fakeDBTX{err: err})

	return &Server{
		queries: queries,
		wsHub:   NewWebSocketHub(queries),
	}
}

// Every /admin/api/ route must 401 an unauthenticated request.
// We previously erroneously redirected.
func TestAdminAPIRejectsUnauthenticatedWithJSON(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	mux := http.NewServeMux()
	app.registerAdminAPI(mux, newAPIRoutes())

	for _, route := range []struct {
		method string
		target string
	}{
		{http.MethodGet, "/admin/api/courses"},
		{http.MethodPost, "/admin/api/courses"},
		{http.MethodPut, "/admin/api/courses/SWIM"},
		{http.MethodDelete, "/admin/api/courses/SWIM"},
		{http.MethodGet, "/admin/api/students"},
		{http.MethodPut, "/admin/api/students"},
		{http.MethodDelete, "/admin/api/students/s1"},
		{http.MethodPost, "/admin/api/students/s1/session"},
		{http.MethodGet, "/admin/api/enrollments"},
		{http.MethodPost, "/admin/api/enrollments"},
		{http.MethodPut, "/admin/api/enrollments/policy"},
		{http.MethodDelete, "/admin/api/enrollments"},
		{http.MethodGet, "/admin/api/grades"},
		{http.MethodPost, "/admin/api/grades/order"},
		{http.MethodPut, "/admin/api/grades/Y9/window"},
		{http.MethodPut, "/admin/api/grades/Y9/budget"},
		{http.MethodPut, "/admin/api/grades/Y9/requirements"},
		{http.MethodGet, "/admin/api/periods"},
		{http.MethodPost, "/admin/api/periods/order"},
		{http.MethodGet, "/admin/api/categories"},
		{http.MethodPost, "/admin/api/categories"},
		{http.MethodGet, "/admin/api/enrollments/export"},
		{http.MethodPost, "/admin/api/students/import"},
		{http.MethodDelete, "/admin/api/data/students"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), route.method, route.target, nil))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.target, rec.Code, http.StatusUnauthorized)

			continue
		}

		if loc := rec.Header().Get("Location"); loc != "" {
			t.Errorf("%s %s: redirected to %q; API routes must never redirect", route.method, route.target, loc)
		}

		if body := decodeErrorBody(t, rec); body.Error.Code != codeUnauthenticated {
			t.Errorf("%s %s: code = %q, want %q", route.method, route.target, body.Error.Code, codeUnauthenticated)
		}
	}
}

// Same here.
func TestStudentAPIRejectsUnauthenticatedWithJSON(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	handler := app.studentAPI("handleStuAPICourses", app.handleStuAPICourses)

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/student/api/courses", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	if body := decodeErrorBody(t, rec); body.Error.Code != codeUnauthenticated {
		t.Errorf("code = %q, want %q", body.Error.Code, codeUnauthenticated)
	}
}

// Through app.router(), not a bare mux built from registerAdminAPI.
//
// The difference is the whole test. A bare mux 405s by itself, because
// the only patterns in it are the API's; the real router also carries
// "/admin/", which matches every one of these paths under every
// method, so the mux stops synthesizing anything and the API's own
// catch-all has to. An earlier version of this test built the bare mux
// and passed while the real router answered 404 with no Allow to every
// wrong method on every wildcard route — which is most of the admin
// API.
func TestAdminAPIRoutesAreMethodScoped(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	router := app.router()

	for _, route := range []struct {
		method string
		target string
		allow  string
	}{
		{http.MethodDelete, "/admin/api/courses", "GET, POST"},
		{http.MethodPost, "/admin/api/enrollments/export", "GET"},
		{http.MethodGet, "/admin/api/students/import", "POST"},
		{http.MethodPost, "/admin/api/students", "GET, PUT"},
		// The wildcards, which is what the bare mux could not see.
		{http.MethodGet, "/admin/api/grades/Y9/window", "PUT"},
		{http.MethodPatch, "/admin/api/courses/ABC", "DELETE, PUT"},
		// A literal path sharing a prefix with a wildcard sibling. Each
		// must report its own methods: "status" is a real endpoint, not
		// a student whose id happens to be "status", and DELETE on it
		// really does reach the {id} route.
		{http.MethodPatch, "/admin/api/students/status", "GET"},
		{http.MethodPatch, "/admin/api/students/s1", "DELETE"},
		{http.MethodGet, "/admin/api/students/s1/session", "POST"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), route.method, route.target, nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.target, rec.Code, http.StatusMethodNotAllowed)

			continue
		}

		// RFC 9110 requires it, and without it a client cannot tell a
		// wrong method from a wrong path.
		if got := rec.Header().Get("Allow"); got != route.allow {
			t.Errorf("%s %s: Allow = %q, want %q", route.method, route.target, got, route.allow)
		}
	}
}

// The other half: a path under an API subtree that belongs to no
// pattern at all is a 404, and it is JSON rather than the SPA.
func TestUnknownAPIPathsAre404JSON(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	router := app.router()

	for _, target := range []string{
		"/admin/api/nope",
		"/admin/api/grades/Y9/nope",
		"/student/api/nope",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want %d", target, rec.Code, http.StatusNotFound)

			continue
		}

		if got := rec.Header().Get("Allow"); got != "" {
			t.Errorf("GET %s: Allow = %q on a 404; no method would have helped", target, got)
		}

		if body := decodeErrorBody(t, rec); body.Error.Code != codeNotFound {
			t.Errorf("GET %s: code = %q, want %q", target, body.Error.Code, codeNotFound)
		}
	}
}

func TestAdminWriteSurfacesRuleRejections(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		sqlstate   string
		wantStatus int
		wantCode   string
	}{
		{"already enrolled", "23505", http.StatusConflict, codeConflict},
		{"unknown grade", "23503", http.StatusConflict, codeConflict},
		{"ill-formed identifier", "23514", http.StatusBadRequest, codeBadRequest},
		{"window closed", "YKG01", http.StatusForbidden, codeWindowClosed},
		{"unexpected failure", "42P01", http.StatusInternalServerError, codeInternal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(&pgconn.PgError{Code: tt.sqlstate, Message: tt.name})
			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/api/students", strings.NewReader(
				`{"students": [{"id": "s1", "name": "Alice", "grade_id": "Y9", "legal_sex": "F"}], "accept": []}`))

			app.apiStudentsUpsert(rec, r, &UserInfoAdmin{Username: "admin"})

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}

			if body := decodeErrorBody(t, rec); body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestAdminWriteSucceedsWithNoContent(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/api/students", strings.NewReader(
		`{"students": [{"id": "s1", "name": "Alice", "grade_id": "Y9", "legal_sex": "F"}], "accept": []}`))

	app.apiStudentsUpsert(rec, r, &UserInfoAdmin{Username: "admin"})

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestAdminWriteRejectsBadPayloads(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		body string
	}{
		{"not JSON", `not json`},
		{"no students", `{"students": [], "accept": []}`},
		{"misspelled field", `{"students": [{"id": "s1", "name": "Alice", "grade_id": "Y9", "leagl_sex": "M"}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/api/students", strings.NewReader(tt.body))

			app.apiStudentsUpsert(rec, r, &UserInfoAdmin{Username: "admin"})

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			if body := decodeErrorBody(t, rec); body.Error.Code != codeBadRequest {
				t.Errorf("code = %q, want %q", body.Error.Code, codeBadRequest)
			}
		})
	}
}

// The unvalidated fields are deliberately unvalidated: the grammars,
// the enums and the references are the database's, and restating them
// here would be a second copy to keep in step. What is checked in Go
// is only what the database cannot see.
func TestCourseInputValidatesOnlyWhatSQLCannot(t *testing.T) {
	t.Parallel()

	negative := int64(-1)

	//exhaustruct:ignore
	if message := (courseInput{MaxStudents: &negative}).validate(); message == "" {
		t.Error("a negative capacity was accepted")
	}

	// No cap at all is a real setting, not a missing one.
	//exhaustruct:ignore
	if message := (courseInput{MaxStudents: nil}).validate(); message != "" {
		t.Errorf("an uncapped course was refused: %q", message)
	}

	// An ill-formed id, an unknown category and a blank name all pass
	// here and are refused by the database, with its message.
	//exhaustruct:ignore
	if message := (courseInput{ID: "not a valid id", Name: "", CategoryID: "NOPE"}).validate(); message != "" {
		t.Errorf("Go restated a database rule: %q", message)
	}
}

// Placement names one course, because that is the unit the database
// locks and judges as a whole.
func TestPlacementInputRequiresACourseAndStudents(t *testing.T) {
	t.Parallel()

	//exhaustruct:ignore
	if message := (placementInput{StudentIDs: []string{"s1"}}).validate(); message == "" {
		t.Error("a placement with no course was accepted")
	}

	//exhaustruct:ignore
	if message := (placementInput{CourseID: "SWIM"}).validate(); message == "" {
		t.Error("a placement with no students was accepted")
	}

	//exhaustruct:ignore
	if message := (placementInput{CourseID: "SWIM", StudentIDs: []string{"s1"}}).validate(); message != "" {
		t.Errorf("a well-formed placement was rejected: %q", message)
	}
}

// Rows are grouped by course because place_enrollments takes one, and
// the groups are ordered by course id because a transaction making
// several calls must take its course locks in one order however the
// spreadsheet was sorted.
func TestGroupEnrollmentRows(t *testing.T) {
	t.Parallel()

	groups, err := groupEnrollmentRows([][]string{
		{"SWIM", "s1", "true", "true"},
		{"SWIM", "s2", "true", "true"},
		{"ART", "s3", "false", "false"},
	})
	if err != nil {
		t.Fatalf("group: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}

	if groups[0].courseID != "ART" || groups[1].courseID != "SWIM" {
		t.Fatalf("groups are not in course order: %q then %q", groups[0].courseID, groups[1].courseID)
	}

	// Within a course the spreadsheet's order survives: placement is
	// an ordered batch and earlier rows win a contested seat.
	if !slices.Equal(groups[1].studentIDs, []string{"s1", "s2"}) {
		t.Errorf("student order not preserved: %v", groups[1].studentIDs)
	}

	if groups[0].droppable || groups[0].budgeted {
		t.Errorf("policy bits not carried: %+v", groups[0])
	}
}

// A policy change mid-file starts a new group, or rows would silently
// inherit the previous row's bits.
func TestGroupEnrollmentRowsSplitsOnPolicy(t *testing.T) {
	t.Parallel()

	groups, err := groupEnrollmentRows([][]string{
		{"SWIM", "s1", "true", "true"},
		{"SWIM", "s2", "false", "true"},
	})
	if err != nil {
		t.Fatalf("group: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
}

func TestGroupEnrollmentRowsNamesTheBadRow(t *testing.T) {
	t.Parallel()

	_, err := groupEnrollmentRows([][]string{
		{"SWIM", "s1", "true", "true"},
		{"SWIM", "s2", "maybe", "true"},
	})
	if err == nil {
		t.Fatal("a non-boolean policy cell was accepted")
	}

	// Row 3: the header is row 1 and the good data row is row 2.
	if !strings.Contains(err.Error(), "row 3") {
		t.Errorf("error does not name the spreadsheet row: %v", err)
	}
}

// A spreadsheet's list cells arrive with stray spaces and, often
// enough, a trailing comma.
func TestSplitList(t *testing.T) {
	t.Parallel()

	if got := splitList(""); got != nil {
		t.Errorf("empty cell = %v, want nil", got)
	}

	if got := splitList(" MON1 , TUE1 ,"); !slices.Equal(got, []string{"MON1", "TUE1"}) {
		t.Errorf("got %v", got)
	}
}

func TestParseBoolCell(t *testing.T) {
	t.Parallel()

	for _, cell := range []string{"", "0", "false", "FALSE", "no", "N"} {
		if v, err := parseBoolCell(cell); err != nil || v {
			t.Errorf("%q = %v, %v; want false", cell, v, err)
		}
	}

	for _, cell := range []string{"1", "true", "TRUE", "yes", "Y"} {
		if v, err := parseBoolCell(cell); err != nil || !v {
			t.Errorf("%q = %v, %v; want true", cell, v, err)
		}
	}

	if _, err := parseBoolCell("perhaps"); err == nil {
		t.Error("a non-boolean cell was accepted")
	}
}
