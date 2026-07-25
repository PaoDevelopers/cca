package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

type gradeAccessPayload struct {
	GradeIDs []string `json:"grade_ids"`
	Enabled  bool     `json:"enabled"`
}

type gradeSchedulePayload struct {
	GradeIDs        []string   `json:"grade_ids"`
	OpensAt         time.Time  `json:"opens_at"`
	ClosesAt        *time.Time `json:"closes_at"`
	ReplaceExisting bool       `json:"replace_existing"`
}

func (app *App) handleAPIAdminGradeAccess(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var payload gradeAccessPayload
	if err := decodeAPIJSON(w, r, &payload); err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
		return
	}
	payload.GradeIDs = normalizeGradeIDs(payload.GradeIDs)
	if len(payload.GradeIDs) == 0 {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "grades_required", "Choose at least one grade.", nil)
		return
	}
	updated, cancelled, err := app.setGradeSelectionAccess(r.Context(), payload.GradeIDs, payload.Enabled)
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.writeJSON(r, w, http.StatusOK, map[string]int64{
		"updated_count":            updated,
		"cancelled_schedule_count": cancelled,
	})
}

func validateGradeSchedulePayload(payload gradeSchedulePayload) (gradeSchedulePayload, error) {
	payload.GradeIDs = normalizeGradeIDs(payload.GradeIDs)
	if len(payload.GradeIDs) == 0 {
		return payload, &validationError{message: "Choose at least one grade."}
	}
	if payload.OpensAt.IsZero() {
		return payload, &validationError{message: "Choose an opening date and time."}
	}
	if payload.ClosesAt != nil && !payload.ClosesAt.After(payload.OpensAt) {
		return payload, &validationError{message: "Closing time must be after opening time."}
	}
	return payload, nil
}

func (app *App) handleAPIAdminGradeSchedules(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	if r.Method != http.MethodPost {
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	var payload gradeSchedulePayload
	if err := decodeAPIJSON(w, r, &payload); err != nil {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
		return
	}
	payload, err := validateGradeSchedulePayload(payload)
	if err != nil {
		app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
		return
	}
	batchID, err := app.saveGradeSelectionSchedule(
		r.Context(), nil, payload.GradeIDs, payload.OpensAt, payload.ClosesAt, payload.ReplaceExisting,
	)
	if err != nil {
		app.writeClassifiedAPIError(r, w, err)
		return
	}
	app.wsHub.Broadcast(WSMessage("invalidate_grades"))
	app.writeJSON(r, w, http.StatusCreated, map[string]int64{"batch_id": batchID})
}

func (app *App) handleAPIAdminGradeSchedule(w http.ResponseWriter, r *http.Request, _ *UserInfoAdmin) {
	batchID, err := strconv.ParseInt(r.PathValue("batch_id"), 10, 64)
	if err != nil || batchID <= 0 {
		app.writeAPIError(r, w, http.StatusBadRequest, "invalid_schedule_id", "Invalid schedule ID.", err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var payload gradeSchedulePayload
		if err := decodeAPIJSON(w, r, &payload); err != nil {
			app.writeAPIError(r, w, http.StatusBadRequest, "invalid_json", err.Error(), err)
			return
		}
		payload, err = validateGradeSchedulePayload(payload)
		if err != nil {
			app.writeAPIError(r, w, http.StatusUnprocessableEntity, "validation_error", err.Error(), err)
			return
		}
		if _, err := app.saveGradeSelectionSchedule(
			r.Context(), &batchID, payload.GradeIDs, payload.OpensAt, payload.ClosesAt, payload.ReplaceExisting,
		); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if _, err := app.cancelGradeSelectionSchedule(r.Context(), batchID); err != nil {
			app.writeClassifiedAPIError(r, w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		app.writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}
