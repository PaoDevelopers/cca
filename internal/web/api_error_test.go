package web //nolint:testpackage

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Fails the test if the body is not an error envelope.
func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) apiErrorBody {
	t.Helper()

	var body apiErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not an error envelope: %v (body %q)", err, rec.Body.String())
	}

	return body
}

// Each SQLSTATE the schema raises has to arrive as its own code: the
// two gates have different remedies (wait, versus ask an
// administrator), and a bad request must not look like a conflict.
func TestDBErrorDetail(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		sqlstate    string
		wantStatus  int
		wantCode    string
		description string
	}{
		{"YKG01", http.StatusForbidden, codeWindowClosed, "window closed"},
		{"YKG02", http.StatusForbidden, codeInviteOnly, "invite only"},
		{"YKG03", http.StatusForbidden, codeNotDroppable, "not droppable"},
		{"23514", http.StatusBadRequest, codeBadRequest, "check violation"},
		{"22P02", http.StatusBadRequest, codeBadRequest, "bad enum value"},
		{"23505", http.StatusConflict, codeConflict, "unique violation"},
		{"23503", http.StatusConflict, codeConflict, "foreign key violation"},
		{"22023", http.StatusBadRequest, codeBadRequest, "invalid parameter"},
		{"P0002", http.StatusNotFound, codeNotFound, "no data found"},
	} {
		err := &pgconn.PgError{Code: tt.sqlstate, Message: tt.description}

		status, detail, ok := dbErrorDetail(err, false)
		if !ok {
			t.Errorf("%s (%s): not classified", tt.sqlstate, tt.description)

			continue
		}

		if status != tt.wantStatus || detail.Code != tt.wantCode {
			t.Errorf("%s (%s): got %d/%q, want %d/%q", tt.sqlstate, tt.description, status, detail.Code, tt.wantStatus, tt.wantCode)
		}
	}
}

// Our own raises are written for a person and go out as they are; the
// database's built-in ones name domains, constraints and relations, and
// its DETAIL for a failed check contains the whole failing row. None of
// that may reach a user.
func TestBuiltInRejectionsDoNotLeakTheSchema(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		err        *pgconn.PgError
		deleting   bool
		wantSubstr string
	}{
		{
			name: "an identifier that breaks its domain says what the format is",
			err: &pgconn.PgError{
				Code: "23514", ConstraintName: "entity_id_check",
				Message: `value for domain entity_id violates check constraint "entity_id_check"`,
			},
			wantSubstr: "capital letters",
		},
		{
			name: "a student id that breaks its domain names the localpart",
			err: &pgconn.PgError{
				Code: "23514", ConstraintName: "localpart_check",
				Message: `value for domain localpart violates check constraint "localpart_check"`,
			},
			wantSubstr: "s22537",
		},
		{
			name: "blank display text says so",
			err: &pgconn.PgError{
				Code: "23514", ConstraintName: "trimmed_text_check",
				Message: `value for domain trimmed_text violates check constraint "trimmed_text_check"`,
			},
			wantSubstr: "must not be empty",
		},
		{
			name: "a table check does not leak the failing row",
			err: &pgconn.PgError{
				Code: "23514", TableName: "courses",
				ConstraintName: "courses_max_students_check",
				Message:        `new row for relation "courses" violates check constraint "courses_max_students_check"`,
				Detail:         "Failing row contains (E, Emil Chen, Y9, F).",
			},
			wantSubstr: "not valid",
		},
		{
			name: "a duplicate says so plainly",
			err: &pgconn.PgError{
				Code: "23505", ConstraintName: "categories_pkey",
				Message: `duplicate key value violates unique constraint "categories_pkey"`,
			},
			wantSubstr: "already exists",
		},
		{
			name: "deleting something still referenced says it is in use",
			err: &pgconn.PgError{
				Code: "23503", ConstraintName: "courses_category_id_fkey",
				Message: `update or delete on table "categories" violates foreign key constraint "courses_category_id_fkey" on table "courses"`,
			},
			deleting:   true,
			wantSubstr: "still in use",
		},
		{
			name: "naming something absent says that instead",
			err: &pgconn.PgError{
				Code: "23503", ConstraintName: "courses_category_id_fkey",
				Message: `insert or update on table "courses" violates foreign key constraint "courses_category_id_fkey"`,
			},
			wantSubstr: "does not exist",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, detail, ok := dbErrorDetail(tt.err, tt.deleting)
			if !ok {
				t.Fatalf("not classified")
			}

			if !strings.Contains(detail.Message, tt.wantSubstr) {
				t.Errorf("message = %q, want it to contain %q", detail.Message, tt.wantSubstr)
			}

			for _, leak := range []string{
				"constraint", "domain", "relation", "_check", "_fkey",
				"_pkey", "Failing row", tt.err.ConstraintName,
			} {
				if leak != "" && strings.Contains(detail.Message, leak) {
					t.Errorf("message leaks %q: %q", leak, detail.Message)
				}
			}
		})
	}
}

// The three gates a student can hit are raised only by the self_*
// functions, so their audience is always a twelve-year-old. Their
// database messages are written for a log reader: they name the
// student to themselves and name courses by internal id. Neither
// belongs on a screen.
func TestTheStudentGatesDoNotShowDatabaseProse(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		state string
		raw   string
		want  string
	}{
		{
			state: "YKG01",
			raw:   "enrollment window is closed for student s22537",
			want:  "year group",
		},
		{
			state: "YKG02",
			raw:   "course CLUB is invite-only",
			want:  "invitation only",
		},
		{
			state: "YKG03",
			raw:   "enrollment of s22537 in SWIM is not droppable",
			want:  "arranged for you",
		},
	} {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()

			//exhaustruct:ignore
			_, detail, ok := dbErrorDetail(&pgconn.PgError{
				Code: tt.state, Message: tt.raw,
			}, false)
			if !ok {
				t.Fatalf("%s was not classified", tt.state)
			}

			if !strings.Contains(detail.Message, tt.want) {
				t.Errorf("message = %q, want it to contain %q",
					detail.Message, tt.want)
			}

			// Nothing of the database's own sentence survives: not the
			// student's id, not the course's, not the phrasing.
			for _, leak := range []string{
				"s22537", "SWIM", "CLUB", "enrollment window is closed",
				"is not droppable", "invite-only",
			} {
				if strings.Contains(detail.Message, leak) {
					t.Errorf("%s leaks %q: %q", tt.state, leak, detail.Message)
				}
			}
		})
	}
}

// The same for the batch payload: the database puts SQLERRM in each
// element, which names the same internals.
func TestMalformedElementsDoNotLeakTheSchema(t *testing.T) {
	t.Parallel()

	_, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:    "YKD01",
		Message: "2 malformed element(s)",
		Detail: `[{"index":3,"id":"lower case","sqlstate":"23514",` +
			`"constraint":"entity_id_check",` +
			`"message":"value for domain entity_id violates check constraint ` + `\"entity_id_check\""},` +
			`{"index":7,"id":"OK","sqlstate":"23503",` +
			`"constraint":"courses_category_id_fkey",` +
			`"message":"insert or update on table ` + `\"courses\" violates foreign key constraint \"courses_category_id_fkey\""}]`,
	}, false)
	if !ok {
		t.Fatal("not classified")
	}

	if len(detail.Malformed) != 2 {
		t.Fatalf("malformed = %d, want 2", len(detail.Malformed))
	}

	// The index and id survive, because that is how a row is found.
	if detail.Malformed[0].Index != 3 || detail.Malformed[0].ID != "lower case" {
		t.Errorf("element identity lost: %+v", detail.Malformed[0])
	}

	if !strings.Contains(detail.Malformed[0].Message, "capital letters") {
		t.Errorf("element 3 message = %q", detail.Malformed[0].Message)
	}

	if !strings.Contains(detail.Malformed[1].Message, "does not exist") {
		t.Errorf("element 7 message = %q", detail.Malformed[1].Message)
	}

	for _, element := range detail.Malformed {
		if element.Constraint != "" {
			t.Errorf("the constraint name was sent to the client: %q", element.Constraint)
		}

		if element.Column != "" {
			t.Errorf("the database column name was sent to the client: %q", element.Column)
		}

		for _, leak := range []string{"_check", "_fkey", "domain", "constraint"} {
			if strings.Contains(element.Message, leak) {
				t.Errorf("element message leaks %q: %q", leak, element.Message)
			}
		}
	}
}

// A transient collision is not the administrator's fault, so it must
// not carry the database's own wording about deadlocks.
func TestDBErrorDetailRewritesTransientCollisions(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"40001", "40P01"} {
		status, detail, ok := dbErrorDetail(&pgconn.PgError{
			Code: state, Message: "deadlock detected",
		}, false)
		if !ok || status != http.StatusConflict || detail.Code != codeConflict {
			t.Errorf("%s: got %d/%q ok=%v", state, status, detail.Code, ok)
		}

		if strings.Contains(detail.Message, "deadlock") {
			t.Errorf("%s: message leaks the database wording: %q", state, detail.Message)
		}
	}
}

// YKV01 exists to be acted on, not only displayed: the payload is what
// the confirm dialog lists and what the retry sends back.
func TestDBErrorDetailCarriesViolations(t *testing.T) {
	t.Parallel()

	status, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:    "YKV01",
		Message: "2 unaccepted violation(s)",
		Detail: `[{"student_id":"s1","rule":"clash","code":"clash:s1:A:B:MON1",` +
			`"other_course_id":"B","period_id":"MON1","detail":"clashes with B in MON1"},` +
			`{"student_id":"s1","rule":"budget","code":"budget:s1",` +
			`"other_course_id":null,"period_id":null,"detail":"would occupy 5 of 4"}]`,
	}, false)
	if !ok || status != http.StatusConflict || detail.Code != codeViolations {
		t.Fatalf("got %d/%q ok=%v", status, detail.Code, ok)
	}

	if len(detail.Violations) != 2 {
		t.Fatalf("violations = %d, want 2", len(detail.Violations))
	}

	if detail.Violations[0].Code != "clash:s1:A:B:MON1" {
		t.Errorf("first code = %q", detail.Violations[0].Code)
	}

	// Nulls stay absent rather than becoming empty strings, so the
	// dialog can tell "no other course" from "a course named \"\"".
	if detail.Violations[1].OtherCourseID != nil {
		t.Errorf("null other_course_id decoded as %q", *detail.Violations[1].OtherCourseID)
	}
}

// YKD01 likewise: without the indices an administrator cannot find the
// rows in a file with a thousand of them.
func TestDBErrorDetailCarriesMalformedElements(t *testing.T) {
	t.Parallel()

	status, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:    "YKD01",
		Message: "1 malformed element(s)",
		Detail:  `[{"index":47,"id":"s99","sqlstate":"23503","message":"grade XX not found"}]`,
	}, false)
	if !ok || status != http.StatusBadRequest || detail.Code != codeMalformed {
		t.Fatalf("got %d/%q ok=%v", status, detail.Code, ok)
	}

	if len(detail.Malformed) != 1 || detail.Malformed[0].Index != 47 {
		t.Fatalf("malformed = %+v", detail.Malformed)
	}
}

// A payload the handler cannot decode is a defect in the schema, not a
// dialog with nothing in it: falling back to internal makes it show up
// in the logs rather than as an empty confirm box.
func TestDBErrorDetailRejectsAnUndecodablePayload(t *testing.T) {
	t.Parallel()

	for _, detail := range []string{"", "not json", "[]"} {
		if _, _, ok := dbErrorDetail(&pgconn.PgError{
			Code: "YKV01", Message: "x", Detail: detail,
		}, false); ok {
			t.Errorf("DETAIL %q was accepted as a violation payload", detail)
		}
	}
}

// An unclassified error must fall through to internal rather than
// being reported as a rule the administrator could have satisfied.
func TestDBErrorDetailIgnoresUnknownErrors(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		&pgconn.PgError{Code: "42P01", Message: "relation does not exist"},
		http.ErrBodyNotAllowed,
	} {
		if _, _, ok := dbErrorDetail(err, false); ok {
			t.Errorf("dbErrorDetail(%v) classified an error it should not have", err)
		}
	}
}

// The import paths wrap database errors with context on the way out,
// so classification has to see through a %w chain. If it stopped doing
// so, every rejection from those paths would silently become a 500.
func TestDBErrorDetailSeesThroughWrapping(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("import row 4: %w",
		fmt.Errorf("place enrollments: %w",
			&pgconn.PgError{Code: "P0002", Message: "course BB not found"}))

	status, detail, ok := dbErrorDetail(err, false)
	if !ok || status != http.StatusNotFound || detail.Code != codeNotFound {
		t.Errorf("got %d/%q ok=%v, want %d/%q", status, detail.Code, ok, http.StatusNotFound, codeNotFound)
	}

	if detail.Message != "course BB not found" {
		t.Errorf("message = %q, want the database's own message", detail.Message)
	}
}

func TestAPIErrorWritesEnvelope(t *testing.T) {
	t.Parallel()

	app := &Server{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/api/courses", nil)

	app.apiError(r, rec, http.StatusConflict, codeConflict, "Course A clashes with B", nil)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	body := decodeErrorBody(t, rec)
	if body.Error.Code != codeConflict {
		t.Errorf("code = %q, want %q", body.Error.Code, codeConflict)
	}

	if body.Error.Message != "Course A clashes with B" {
		t.Errorf("message = %q, want the message passed in", body.Error.Message)
	}
}

// apiDBError is what every write handler calls, so its fallback decides
// whether an unexpected failure looks like a client error.
func TestAPIDBErrorFallsBackToInternal(t *testing.T) {
	t.Parallel()

	app := &Server{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/api/students", nil)

	app.apiDBError(r, rec, &pgconn.PgError{Code: "42P01", Message: "relation does not exist"})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if body := decodeErrorBody(t, rec); body.Error.Code != codeInternal {
		t.Errorf("code = %q, want %q", body.Error.Code, codeInternal)
	}
}

// The violation payload has to survive all the way to the wire: the
// dialog reads it from the response body, not from the log.
func TestAPIDBErrorSerializesViolations(t *testing.T) {
	t.Parallel()

	app := &Server{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/api/courses/SWIM", nil)

	app.apiDBError(r, rec, &pgconn.PgError{
		Code:    "YKV01",
		Message: "1 unaccepted violation(s)",
		Detail: `[{"student_id":"s1","rule":"capacity","code":"capacity:s1:SWIM",` +
			`"other_course_id":null,"period_id":null,"detail":"SWIM is full (3/2)"}]`,
	})

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	body := decodeErrorBody(t, rec)
	if body.Error.Code != codeViolations {
		t.Fatalf("code = %q, want %q", body.Error.Code, codeViolations)
	}

	if len(body.Error.Violations) != 1 || body.Error.Violations[0].Code != "capacity:s1:SWIM" {
		t.Fatalf("violations did not survive encoding: %+v", body.Error.Violations)
	}
}

// And the field is absent, not null or empty, when there is nothing to
// accept: a client may test for the field as well as for the code.
func TestAPIErrorOmitsTheViolationFieldWhenEmpty(t *testing.T) {
	t.Parallel()

	app := &Server{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/admin/api/students", nil)

	app.apiError(r, rec, http.StatusNotFound, codeNotFound, "no such student", nil)

	if strings.Contains(rec.Body.String(), "violations") {
		t.Errorf("empty violations were serialized: %s", rec.Body.String())
	}
}

// The schema raises 22023 fifteen times, always with a sentence
// written for the person who will read it. Bucketing it with the
// built-in rejections replaced every one of them with "That value is
// not valid here": the refusal still happened, and the administrator
// was told nothing they could act on. set_period_order refusing an
// incomplete list is the case that matters — a stale tab silently
// dropping a period is exactly what it exists to catch, and the whole
// point is saying so.
func TestOurOwnParameterRejectionsKeepTheirWording(t *testing.T) {
	t.Parallel()

	for _, message := range []string{
		"the order must name every period exactly once",
		"requirement must name at least one category",
		"parameter arrays must have equal lengths",
	} {
		//exhaustruct:ignore
		status, detail, ok := dbErrorDetail(&pgconn.PgError{
			Code: "22023", Message: message,
		}, false)

		if !ok || status != http.StatusBadRequest {
			t.Errorf("%q: status = %d, ok = %v; want %d, true", message, status, ok, http.StatusBadRequest)

			continue
		}

		if detail.Message != message {
			t.Errorf("message = %q, want the schema's own %q", detail.Message, message)
		}
	}
}

// The other half of the same rule: PostgreSQL's own wording never
// reaches a user, because it names domains and constraints.
func TestBuiltInRejectionsAreRewritten(t *testing.T) {
	t.Parallel()

	//exhaustruct:ignore
	_, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:           "23514",
		Message:        `value for domain count_value violates check constraint "count_value_check"`,
		ConstraintName: "count_value_check",
	}, false)

	if !ok {
		t.Fatal("a domain rejection was not classified")
	}

	if strings.Contains(detail.Message, "domain") || strings.Contains(detail.Message, "constraint") {
		t.Errorf("message = %q; it repeats the database's own wording", detail.Message)
	}

	// And says what the range actually is, which is the one thing the
	// person needs in order to fix it.
	if !strings.Contains(detail.Message, "1000000") {
		t.Errorf("message = %q; it does not say what is allowed", detail.Message)
	}
}

// The column an element was rejected on is the half a domain rejection
// cannot supply: it is raised while casting, so it names the domain and
// not the column. The write function names the column instead, and a
// not-null violation — the one rejection that does name its own column
// — overrides it, because that is the one case the statement boundary
// cannot pin down.
func TestMalformedElementsNameTheirColumn(t *testing.T) {
	t.Parallel()

	_, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:    "YKD01",
		Message: "3 malformed element(s)",
		Detail: `[{"index":1,"id":"1234","sqlstate":"23514",` +
			`"constraint":"trimmed_text_check","field":"term",` +
			`"message":"value for domain trimmed_text violates check constraint"},` +
			`{"index":2,"id":"1234","sqlstate":"23502",` +
			`"field":"category","column":"name",` +
			`"message":"null value in column"},` +
			`{"index":3,"id":"1234","sqlstate":"22P02",` +
			`"field":"allowed_legal_sexes",` +
			`"message":"invalid input value for enum legal_sex"}]`,
	}, false)
	if !ok {
		t.Fatal("not classified")
	}

	want := []string{"term", "name", "allowed_legal_sexes"}
	for i, field := range want {
		if detail.Malformed[i].Field != field {
			t.Errorf("element %d field = %q, want %q",
				i, detail.Malformed[i].Field, field)
		}
	}
}

// An element the write function could not attribute carries no column,
// and the message must still stand on its own rather than inventing one.
func TestMalformedElementsSurviveWithoutAColumn(t *testing.T) {
	t.Parallel()

	_, detail, ok := dbErrorDetail(&pgconn.PgError{
		Code:    "YKD01",
		Message: "1 malformed element(s)",
		Detail: `[{"index":9,"id":"OK","sqlstate":"23502",` +
			`"column":"no_such_column","message":"null value in column"}]`,
	}, false)
	if !ok {
		t.Fatal("not classified")
	}

	if detail.Malformed[0].Field != "" {
		t.Errorf("field = %q, want empty", detail.Malformed[0].Field)
	}

	if !strings.Contains(detail.Malformed[0].Message, "required value is missing") {
		t.Errorf("message = %q", detail.Malformed[0].Message)
	}
}
