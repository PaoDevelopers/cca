package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type apiErrorBody struct {
	Error apiErrorDetail `json:"error"`
}

type apiErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (app *App) writeAPIError(r *http.Request, w http.ResponseWriter, status int, code, message string, err error, extra ...slog.Attr) {
	attrs := append(extra, slog.String("api_error_code", code))
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	app.apiError(r, w, status, apiErrorBody{Error: apiErrorDetail{
		Code:    code,
		Message: message,
	}}, attrs...)
}

func (app *App) writeClassifiedAPIError(r *http.Request, w http.ResponseWriter, err error, extra ...slog.Attr) {
	status, code, message := classifyAPIError(err)
	app.writeAPIError(r, w, status, code, message, err, extra...)
}

func classifyAPIError(err error) (int, string, string) {
	if errors.Is(err, errCourseNeedsPeriod) {
		return http.StatusUnprocessableEntity, "course_requires_period", "A course must have at least one timetable period."
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound, "not_found", "The requested record no longer exists."
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "choices_period_conflict":
			return http.StatusConflict, "schedule_conflict", "This selection clashes with an existing CCA."
		case "course_requires_period":
			return http.StatusUnprocessableEntity, "course_requires_period", "This CCA does not have a timetable."
		case "choices_membership":
			return http.StatusUnprocessableEntity, "invite_only", "This CCA is invitation only."
		case "choices_legal_sex":
			return http.StatusUnprocessableEntity, "legal_sex_restricted", "This CCA is not available for you."
		case "choices_grade":
			return http.StatusUnprocessableEntity, "grade_restricted", "This CCA is not available for this grade."
		case "choices_window":
			return http.StatusUnprocessableEntity, "selections_closed", "Selections are closed for this grade."
		case "choices_force_locked":
			return http.StatusConflict, "forced_selection", "A forced selection can only be removed by an administrator."
		case "choices_max_own":
			return http.StatusUnprocessableEntity, "choice_limit", "The student has reached the own-selection limit."
		case "choices_capacity":
			return http.StatusConflict, "course_full", "This CCA is full."
		case "course_period_in_use", "course_periods_immutable":
			return http.StatusConflict, "course_period_in_use", "Remove selections from this time slot before changing it."
		case "course_periods_period_id_fkey":
			return http.StatusUnprocessableEntity, "invalid_period", "Use one of the fixed Monday-through-Thursday CCA slots."
		case "choices_course_period", "choices_course_period_fkey":
			return http.StatusUnprocessableEntity, "invalid_course_period", "Choose a timetable slot offered by this CCA."
		case "periods_fixed":
			return http.StatusConflict, "periods_fixed", "Timetable periods are fixed and cannot be changed."
		case "reset_selections_required":
			return http.StatusConflict, "reset_selections_required", "Reset selections before resetting courses or students."
		case "reset_scope":
			return http.StatusUnprocessableEntity, "invalid_reset_scope", "Choose selections, courses, or students."
		case "grade_schedule_exists":
			return http.StatusConflict, "grade_schedule_exists", "One or more grades already have a selection schedule."
		case "grade_schedule_opened", "grade_schedule_opened_conflict":
			return http.StatusConflict, "grade_schedule_opened", "An open selection window can only change its future closing time."
		case "grade_selection_schedule_range":
			return http.StatusUnprocessableEntity, "invalid_schedule_range", "Closing time must be after opening time."
		case "grade_schedule_in_past":
			return http.StatusUnprocessableEntity, "schedule_in_past", "Opening time must be in the future."
		case "grade_access_grades", "grade_schedule_grades", "grade_schedule_opens_at", "grade_schedule_now", "grade_settings":
			return http.StatusUnprocessableEntity, "validation_error", "Complete the required grade schedule fields."
		case "grade_schedule_batch":
			return http.StatusNotFound, "not_found", "The selection schedule no longer exists."
		}
		switch pgErr.Code {
		case "P0002":
			return http.StatusNotFound, "not_found", "The requested record no longer exists."
		case "23505":
			return http.StatusConflict, "already_exists", "That record already exists."
		case "23503":
			return http.StatusConflict, "dependency_conflict", "This record is still referenced by other data."
		case "23514", "22023":
			return http.StatusUnprocessableEntity, "validation_error", "The requested change violates a data rule."
		}
	}
	return http.StatusInternalServerError, "internal_error", "The server could not complete the request."
}

func decodeAPIJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func requestIsSameOrigin(r *http.Request) bool {
	if isSafeMethod(r.Method) {
		return true
	}
	if site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site == "cross-site" {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	} else if forwardedProto := strings.ToLower(headerFirstValue(r.Header.Get("X-Forwarded-Proto"))); forwardedProto == "http" || forwardedProto == "https" {
		// Reverse proxies must overwrite (not append to) X-Forwarded-Proto.
		// Host is intentionally taken from r.Host below; trusting a client-set
		// X-Forwarded-Host would let it redefine the expected origin.
		expectedScheme = forwardedProto
	}
	return strings.EqualFold(u.Scheme, expectedScheme) && strings.EqualFold(u.Host, strings.TrimSpace(r.Host))
}

func (app *App) apiStudentOnly(handlerName string, handler func(http.ResponseWriter, *http.Request, *UserInfoStudent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ui, err := app.authenticateRequest(r)
		if err != nil || ui == nil {
			app.writeAPIError(r, w, http.StatusUnauthorized, "unauthenticated", "Please sign in again.", err)
			return
		}
		student, ok := ui.(*UserInfoStudent)
		if !ok {
			app.writeAPIError(r, w, http.StatusForbidden, "forbidden", "A student account is required.", nil)
			return
		}
		if !requestIsSameOrigin(r) {
			app.writeAPIError(r, w, http.StatusForbidden, "cross_site_request", "Cross-site mutations are not allowed.", nil)
			return
		}
		app.logRequestStart(r, handlerName, slog.Int64("student_id", student.ID))
		handler(w, r, student)
	}
}

func (app *App) apiAdminOnly(handlerName string, handler func(http.ResponseWriter, *http.Request, *UserInfoAdmin)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ui, err := app.authenticateRequest(r)
		if err != nil || ui == nil {
			app.writeAPIError(r, w, http.StatusUnauthorized, "unauthenticated", "Please sign in again.", err)
			return
		}
		admin, ok := ui.(*UserInfoAdmin)
		if !ok {
			app.writeAPIError(r, w, http.StatusForbidden, "forbidden", "An administrator account is required.", nil)
			return
		}
		if !requestIsSameOrigin(r) {
			app.writeAPIError(r, w, http.StatusForbidden, "cross_site_request", "Cross-site mutations are not allowed.", nil)
			return
		}
		app.logRequestStart(r, handlerName, slog.String("admin_username", admin.Username))
		handler(w, r, admin)
	}
}
