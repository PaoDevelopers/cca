package httpapi

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type testAuthView struct {
	Enabled           bool `json:"enabled"`
	RequiresAccessKey bool `json:"requires_access_key"`
}

type testAuthLoginPayload struct {
	Role       string `json:"role"`
	Identifier string `json:"identifier"`
	AccessKey  string `json:"access_key,omitempty"`
}

type testAuthLoginResult struct {
	RedirectTo string `json:"redirect_to"`
}

func (app *App) handleAPITestAuth(w http.ResponseWriter, r *http.Request) {
	if !app.testAuthRequestAllowed(r) {
		app.writeAPIError(r, w, http.StatusNotFound, "not_found", "The requested endpoint does not exist.", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		app.writeJSON(r, w, http.StatusOK, testAuthView{
			Enabled:           true,
			RequiresAccessKey: app.testAuthRequiresAccessKey(),
		})
	case http.MethodPost:
		if !requestIsSameOrigin(r) {
			app.writeAPIError(r, w, http.StatusForbidden, "cross_site_request", "Cross-site test logins are not allowed.", nil)
			return
		}

		var payload testAuthLoginPayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		if app.testAuthRequiresAccessKey() && subtle.ConstantTimeCompare([]byte(payload.AccessKey), []byte(app.config.TestAuth.AccessKey)) != 1 {
			app.writeAPIError(r, w, http.StatusForbidden, "invalid_test_access_key", "The test access key is invalid.", nil)
			return
		}

		payload.Role = strings.TrimSpace(payload.Role)
		payload.Identifier = strings.TrimSpace(payload.Identifier)
		secureCookie := requestUsesHTTPS(r)
		switch payload.Role {
		case "student":
			studentID, err := parseTestStudentID(payload.Identifier)
			if err != nil {
				app.writeAPIError(r, w, http.StatusUnprocessableEntity, "invalid_student_id", "Enter a valid positive student ID.", err)
				return
			}
			if err := app.issueStudentSession(r.Context(), w, studentID, secureCookie); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					app.writeAPIError(r, w, http.StatusForbidden, "invalid_test_account", "That test student does not exist.", err)
					return
				}
				app.writeAPIError(r, w, http.StatusInternalServerError, "session_error", "The test session could not be created.", err)
				return
			}
			app.logInfo(r, "test student authenticated", slog.Int64("student_id", studentID))
			app.writeJSON(r, w, http.StatusOK, testAuthLoginResult{RedirectTo: "/student/"})
		case "admin":
			if payload.Identifier == "" {
				app.writeAPIError(r, w, http.StatusUnprocessableEntity, "invalid_admin_username", "Enter an administrator username.", nil)
				return
			}
			if _, allowed := app.config.Admins[payload.Identifier]; !allowed {
				app.writeAPIError(r, w, http.StatusForbidden, "invalid_test_account", "That test administrator is not configured.", nil)
				return
			}
			if err := app.issueAdminSession(r.Context(), w, payload.Identifier, secureCookie); err != nil {
				app.writeAPIError(r, w, http.StatusInternalServerError, "session_error", "The test session could not be created.", err)
				return
			}
			app.logInfo(r, "test administrator authenticated", slog.String("admin_username", payload.Identifier))
			app.writeJSON(r, w, http.StatusOK, testAuthLoginResult{RedirectTo: "/admin/"})
		default:
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "invalid_test_role", "Choose either student or administrator test login.", nil)
		}
	default:
		w.Header().Set("Allow", "GET, POST")
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (app *App) testAuthRequiresAccessKey() bool {
	return strings.TrimSpace(app.config.TestAuth.AccessKey) != ""
}

func (app *App) handleTestAuthFrontend(w http.ResponseWriter, r *http.Request) {
	if !app.testAuthRequestAllowed(r) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	serveFrontendIndex(w, r)
}

func (app *App) testAuthRequestAllowed(r *http.Request) bool {
	if !app.config.TestAuth.Enabled {
		return false
	}
	if app.config.TestAuth.AllowRemote {
		return true
	}
	return requestIsStrictlyLocal(r)
}

func requestIsStrictlyLocal(r *http.Request) bool {
	if r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}

	host := strings.TrimSpace(r.Host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	hostIsLocal := strings.EqualFold(host, "localhost")
	if ip := net.ParseIP(host); ip != nil {
		hostIsLocal = ip.IsLoopback()
	}
	if !hostIsLocal {
		return false
	}

	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return false
	}
	remoteIP := net.ParseIP(strings.Trim(remoteHost, "[]"))
	return remoteIP != nil && remoteIP.IsLoopback()
}

func requestUsesHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(headerFirstValue(r.Header.Get("X-Forwarded-Proto")), "https")
}

func parseTestStudentID(identifier string) (int64, error) {
	identifier = strings.TrimSpace(identifier)
	if len(identifier) > 0 && (identifier[0] == 's' || identifier[0] == 'S') {
		identifier = identifier[1:]
	}
	studentID, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		return 0, err
	}
	if studentID <= 0 {
		return 0, errors.New("student ID must be positive")
	}
	return studentID, nil
}
