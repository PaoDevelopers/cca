package web //nolint:testpackage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/PaoDevelopers/cca/internal/db"
)

// A database whose every write succeeds on one row, so a delete finds
// the student it was asked to remove.
type oneRowDBTX struct{ fakeDBTX }

func (oneRowDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("DELETE 1"), nil
}

// Editing or deleting a student tells that student's pages and the
// administrators, and nobody else: every student page told re-reads
// its eligibility, and a roster fix mid-window used to send every
// open page in the school back for it.
func TestStudentEditsNotifyOnlyThatStudent(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		call func(t *testing.T, app *Server, w http.ResponseWriter)
		want []WSMessage
	}{
		{
			name: "upsert",
			call: func(t *testing.T, app *Server, w http.ResponseWriter) {
				t.Helper()

				r := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/admin/api/students", strings.NewReader(
					`{"students": [{"id": "s1", "name": "A", "grade_id": "Y9", "legal_sex": "F"}]}`))
				app.apiStudentsUpsert(w, r, &UserInfoAdmin{Username: "e2e.admin"})
			},
			want: []WSMessage{"invalidate_students", "invalidate_enrollments"},
		},
		{
			name: "delete",
			call: func(t *testing.T, app *Server, w http.ResponseWriter) {
				t.Helper()

				r := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/admin/api/students/s1", nil)
				r.SetPathValue("id", "s1")
				app.apiStudentsDelete(w, r, &UserInfoAdmin{Username: "e2e.admin"})
			},
			want: []WSMessage{"invalidate_students"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := testServer(nil)
			app.queries = db.New(oneRowDBTX{})

			edited := newWSClient(app.wsHub, nil, "s1")
			bystander := newWSClient(app.wsHub, nil, "s2")
			admin := newAdminWSClient(app.wsHub, nil, wsAdminKey("e2e.admin"))

			for _, c := range []*Client{edited, bystander, admin} {
				app.wsHub.register(c)
			}

			rec := httptest.NewRecorder()
			tt.call(t, app, rec)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, http.StatusNoContent, rec.Body.String())
			}

			if got := edited.takePending(); !slices.Equal(got, tt.want) {
				t.Errorf("the edited student got %v, want %v", got, tt.want)
			}

			if got := bystander.takePending(); len(got) != 0 {
				t.Errorf("another student was told %v", got)
			}

			if got := admin.takePending(); !slices.Equal(got, tt.want) {
				t.Errorf("the administrator got %v, want %v", got, tt.want)
			}
		})
	}
}
