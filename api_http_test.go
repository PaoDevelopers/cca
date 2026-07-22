package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestRequestIsSameOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		secFetchSite   string
		forwardedProto string
		forwardedHost  string
		want           bool
	}{
		{name: "same origin", origin: "http://cca.example", want: true},
		{name: "different host", origin: "http://attacker.example", want: false},
		{name: "browser reports cross site", origin: "http://cca.example", secFetchSite: "cross-site", want: false},
		{name: "trusted proxy scheme", origin: "https://cca.example", forwardedProto: "https", want: true},
		{name: "forwarded host cannot redefine origin", origin: "http://public.example", forwardedHost: "public.example", want: false},
		{name: "non browser client without origin", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://cca.example/api/v1/admin/courses", nil)
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			r.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			r.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			if got := requestIsSameOrigin(r); got != tt.want {
				t.Fatalf("requestIsSameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyAPIError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "missing row", err: pgx.ErrNoRows, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "schedule conflict", err: &pgconn.PgError{ConstraintName: "choices_period_conflict"}, wantStatus: http.StatusConflict, wantCode: "schedule_conflict"},
		{name: "course needs period", err: &pgconn.PgError{ConstraintName: "course_requires_period"}, wantStatus: http.StatusUnprocessableEntity, wantCode: "course_requires_period"},
		{name: "forced choice", err: &pgconn.PgError{ConstraintName: "choices_force_locked"}, wantStatus: http.StatusConflict, wantCode: "forced_selection"},
		{name: "selection window", err: &pgconn.PgError{ConstraintName: "choices_window"}, wantStatus: http.StatusUnprocessableEntity, wantCode: "selections_closed"},
		{name: "invalid fixed period", err: &pgconn.PgError{ConstraintName: "course_periods_period_id_fkey"}, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_period"},
		{name: "period not offered by course", err: &pgconn.PgError{ConstraintName: "choices_course_period_fkey"}, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_course_period"},
		{name: "course period in use", err: &pgconn.PgError{ConstraintName: "course_period_in_use"}, wantStatus: http.StatusConflict, wantCode: "course_period_in_use"},
		{name: "immutable period catalogue", err: &pgconn.PgError{ConstraintName: "periods_fixed"}, wantStatus: http.StatusConflict, wantCode: "periods_fixed"},
		{name: "grade schedule conflict", err: &pgconn.PgError{ConstraintName: "grade_schedule_exists"}, wantStatus: http.StatusConflict, wantCode: "grade_schedule_exists"},
		{name: "active grade schedule", err: &pgconn.PgError{ConstraintName: "grade_schedule_opened_conflict"}, wantStatus: http.StatusConflict, wantCode: "grade_schedule_opened"},
		{name: "invalid grade schedule range", err: &pgconn.PgError{ConstraintName: "grade_selection_schedule_range"}, wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_schedule_range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, _ := classifyAPIError(tt.err)
			if status != tt.wantStatus || code != tt.wantCode {
				t.Fatalf("classifyAPIError() = (%d, %q), want (%d, %q)", status, code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}
