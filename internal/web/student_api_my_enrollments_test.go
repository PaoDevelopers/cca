package web //nolint:testpackage

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PaoDevelopers/cca/internal/db"
)

func TestStudentEnrollmentWritesNotifyTheStudentsOwnSessions(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		method string
		body   string
	}{
		{"enroll", http.MethodPut, `{"course_id": "BB"}`},
		{"drop", http.MethodDelete, `{"course_id": "BB"}`},
		{"swap", http.MethodPost, `{"course_id": "BB", "replacing": ["CH"]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			student := &UserInfoStudent{ID: "s42"}

			// A second tab belonging to the same student,
			// and an administrator.
			otherSession := newWSClient(app.wsHub, nil, student.ID)
			adminSession := newAdminWSClient(app.wsHub, nil, wsAdminKey("e2e.admin"))
			app.wsHub.register(otherSession)
			app.wsHub.register(adminSession)

			rec := httptest.NewRecorder()
			r := httptest.NewRequestWithContext(
				t.Context(), tt.method, "/student/api/my_enrollments", strings.NewReader(tt.body))

			app.handleStuAPIMyEnrollments(rec, r, student)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
			}

			// The student's other page is sent the new state itself,
			// and nothing that would make it read the state back.
			got := otherSession.takePending()
			if len(got) != 1 || !strings.HasPrefix(string(got[0]), "student_state,") {
				t.Fatalf("the student's other session got %v, want one student_state frame", got)
			}

			var state map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(string(got[0]), "student_state,")), &state); err != nil {
				t.Fatalf("student_state payload is not JSON: %v", err)
			}

			for _, key := range []string{"enrollments", "eligibility", "user"} {
				if _, ok := state[key]; !ok {
					t.Errorf("student_state payload has no %q: %v", key, state)
				}
			}

			const want = WSMessage("invalidate_enrollments")

			if got := adminSession.takePending(); !slices.Contains(got, want) {
				t.Errorf("the administrator session got %v, want it to contain %q", got, want)
			}
		})
	}
}

var errReadsDown = errors.New("reads unavailable")

// A database whose writes succeed and whose reads fail.
type readsFailDBTX struct{ fakeDBTX }

func (readsFailDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (readsFailDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errReadsDown
}

func (readsFailDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return errRow{scanErr: errReadsDown}
}

// If the new state cannot be read after a write, the write has still
// happened, and the student's pages are told to read it themselves.
func TestStudentWriteFallsBackToInvalidateWhenTheStateCannotBeRead(t *testing.T) {
	t.Parallel()

	app := testServer(nil)
	app.queries = db.New(readsFailDBTX{})
	student := &UserInfoStudent{ID: "s42"}

	otherSession := newWSClient(app.wsHub, nil, student.ID)
	app.wsHub.register(otherSession)

	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(
		t.Context(), http.MethodPut, "/student/api/my_enrollments", strings.NewReader(`{"course_id": "BB"}`))

	app.handleStuAPIMyEnrollments(rec, r, student)

	if got := otherSession.takePending(); !slices.Equal(got, []WSMessage{"invalidate_enrollments"}) {
		t.Errorf("the student's other session got %v, want invalidate_enrollments", got)
	}
}

// A page keeps the newest state it has been given: two writes from two
// tabs may finish reading in either order.
func TestAPageKeepsTheNewestStudentState(t *testing.T) {
	t.Parallel()

	hub := NewWebSocketHub(nil)
	page := newWSClient(hub, nil, "s1")
	other := newWSClient(hub, nil, "s2")

	hub.register(page)
	hub.register(other)

	first, second := hub.NextStateGeneration(), hub.NextStateGeneration()

	hub.PushStudentState("s1", second, "student_state,newer")
	hub.PushStudentState("s1", first, "student_state,older")
	hub.BroadcastToStudents([]string{"s1"}, "invalidate_courses")

	// The state goes first, ahead of events queued behind it.
	if got := page.takePending(); !slices.Equal(got, []WSMessage{"student_state,newer", "invalidate_courses"}) {
		t.Errorf("page got %v, want the newer state then the event", got)
	}

	// Only the student it belongs to.
	if got := other.takePending(); len(got) != 0 {
		t.Errorf("another student's page got %v", got)
	}

	// Sent once; the older one does not come back afterwards.
	hub.PushStudentState("s1", first, "student_state,older")

	if got := page.takePending(); len(got) != 0 {
		t.Errorf("an older state was sent after a newer one: %v", got)
	}
}
