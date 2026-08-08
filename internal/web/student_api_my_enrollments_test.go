package web //nolint:testpackage

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
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

			const want = WSMessage("invalidate_enrollments")

			if got := otherSession.takePending(); !slices.Contains(got, want) {
				t.Errorf("the student's other session got %v, want it to contain %q", got, want)
			}

			if got := adminSession.takePending(); !slices.Contains(got, want) {
				t.Errorf("the administrator session got %v, want it to contain %q", got, want)
			}
		})
	}
}
