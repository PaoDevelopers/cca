package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminDashboardRejectsNonGetRequestsBeforeDatabaseAccess(t *testing.T) {
	app := &App{}
	admin := &UserInfoAdmin{Username: "test-admin"}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/api/v1/admin/dashboard", nil)
			w := httptest.NewRecorder()

			app.handleAPIAdminDashboard(w, r, admin)

			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
			if got := w.Header().Get("Allow"); got != http.MethodGet {
				t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
			}
		})
	}
}
