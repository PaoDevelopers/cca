package main

import (
	"log/slog"
	"net/http"
	"strings"
)

type adminResetPayload struct {
	Scope        string `json:"scope"`
	Confirmation string `json:"confirmation"`
}

type adminResetResponse struct {
	Scope            string `json:"scope"`
	DeletedCount     int64  `json:"deleted_count"`
	ClosedGradeCount int64  `json:"closed_grade_count"`
}

var adminResetConfirmations = map[string]string{
	"selections": "RESET SELECTIONS",
	"courses":    "RESET COURSES",
	"students":   "RESET STUDENTS",
}

func adminResetConfirmation(scope string) (string, bool) {
	confirmation, ok := adminResetConfirmations[scope]
	return confirmation, ok
}

func (app *App) handleAPIAdminReset(w http.ResponseWriter, r *http.Request, admin *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}

	var payload adminResetPayload
	if err := decodeAPIJSON(w, r, &payload); err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
		return
	}

	payload.Scope = strings.ToLower(strings.TrimSpace(payload.Scope))
	expectedConfirmation, ok := adminResetConfirmation(payload.Scope)
	if !ok {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "invalid_reset_scope", "Choose selections, courses, or students.", nil)
		return
	}
	if payload.Confirmation != expectedConfirmation {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "confirmation_mismatch", "Enter the confirmation phrase exactly as shown.", nil)
		return
	}

	var result adminResetResponse
	err := app.pool.QueryRow(
		r.Context(),
		`SELECT reset_scope, deleted_count, closed_grade_count FROM admin_reset_data($1)`,
		payload.Scope,
	).Scan(&result.Scope, &result.DeletedCount, &result.ClosedGradeCount)
	if err != nil {
		app.writeClassifiedAPIError(r, w, err, slog.String("reset_scope", payload.Scope))
		return
	}

	app.logInfo(
		r,
		logMsgAdminDataReset,
		slog.String("admin_username", admin.Username),
		slog.String("reset_scope", result.Scope),
		slog.Int64("deleted_count", result.DeletedCount),
		slog.Int64("closed_grade_count", result.ClosedGradeCount),
	)

	// Every reset closes selection windows, and each affected dataset is part
	// of the shared student/admin bootstrap. Broad invalidation keeps all open
	// clients coherent without embedding reset semantics in the UI.
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.wsHub.Broadcast(WSMessage("invalidate_courses"))
	app.wsHub.Broadcast(WSMessage("invalidate_selections"))
	app.writeJSON(r, w, http.StatusOK, result)
}
