package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminPeriodsRejectsMutationsBeforeDatabaseAccess(t *testing.T) {
	app := &App{}
	admin := &UserInfoAdmin{Username: "test-admin"}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/api/v1/admin/periods", nil)
			w := httptest.NewRecorder()

			// A nil query layer is intentional: any attempted period mutation would
			// panic, while the read-only API must reject the method immediately.
			app.handleAPIAdminPeriods(w, r, admin)

			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
			}
			if got := w.Header().Get("Allow"); got != http.MethodGet {
				t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
			}
		})
	}
}
