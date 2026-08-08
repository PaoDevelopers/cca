package web //nolint:testpackage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// The database being away is not the same as the code being wrong.
//
// Both arrive at the same handler as a plain error, and before this was
// separated both became a 500 with "tell an administrator" and an
// error-level log line. That is wrong twice over: the advice is wrong,
// because the remedy is to wait; and the level is wrong, because one
// outage becomes a page for every request in flight, which buries the
// errors that do need a person.
//
// The two shapes below are the ones the real driver produces. They were
// obtained by taking a live database away from a running server and
// printing the error chain, not by reading the driver's source: a
// connection that cannot be made is a pgconn.ConnectError, and a
// database that did not answer inside the ceiling — whether because the
// query was slow or because every pooled connection was busy — is a
// context deadline.
// A stand-in for whatever nobody thought of.
var errUnexpectedForTest = errors.New("something nobody expected")

func TestInfrastructureFailureIsTransientNotInternal(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
	}{
		{
			name: "the database cannot be reached",
			err:  connectError(t),
		},
		{
			name: "the read ceiling fired",
			err:  context.DeadlineExceeded,
		},
		{
			name: "the pool would not hand out a connection in time",
			err:  fmt.Errorf("acquire: %w", context.DeadlineExceeded),
		},
		{
			name: "wrapped, as the handlers wrap it",
			err:  fmt.Errorf("fetch grades: %w", connectError(t)),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin/api/courses", nil)

			app.apiDBError(r, rec, tt.err)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
			}

			body := decodeErrorBody(t, rec)
			if body.Error.Code != codeUnavailable {
				t.Errorf("code = %q, want %q", body.Error.Code, codeUnavailable)
			}

			// The advice has to be "wait", not "report".
			if !strings.Contains(body.Error.Message, "try again") {
				t.Errorf("message does not tell the user to retry: %q", body.Error.Message)
			}

			if strings.Contains(body.Error.Message, "administrator") {
				t.Errorf("message sends the user to an administrator for an outage: %q",
					body.Error.Message)
			}

			// And nothing about where the database lives.
			for _, leak := range []string{"db.example", "5432", "password", "user="} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("response leaks %q: %s", leak, rec.Body.String())
				}
			}
		})
	}
}

// connectError produces the error the driver really makes when it
// cannot reach a database, by failing a connection to a closed port.
// Constructing one by hand is not possible from outside the driver —
// it carries an unexported cause — and would be a guess at its shape
// even if it were.
func connectError(t *testing.T) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, err := pgconn.Connect(ctx, "postgres://127.0.0.1:1/none?sslmode=disable")
	if err == nil {
		t.Fatal("something is listening on port 1; this test needs a closed port")
	}

	if _, ok := errors.AsType[*pgconn.ConnectError](err); !ok {
		t.Fatalf("a refused connection gave %T, not a *pgconn.ConnectError: %v", err, err)
	}

	// Returned as it is: the point of this helper is to hand back the
	// driver's own error, unwrapped and unaltered.
	//nolint:wrapcheck
	return err
}

// The other half: something genuinely unexpected must still be a 500,
// or the separation would just be a way of hiding real faults.
func TestAnUnexpectedFailureIsStillInternal(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		err  error
	}{
		{"an unclassified SQLSTATE", &pgconn.PgError{Code: "42P01", Message: "relation does not exist"}},
		{"a plain error", errUnexpectedForTest},
		{"a network error that is not a connect failure", &net.AddrError{Err: "bad", Addr: "x"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin/api/courses", nil)

			app.apiDBError(r, rec, tt.err)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}

			if body := decodeErrorBody(t, rec); body.Error.Code != codeInternal {
				t.Errorf("code = %q, want %q", body.Error.Code, codeInternal)
			}
		})
	}
}
