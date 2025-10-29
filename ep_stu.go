package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func (app *App) handleStu(w http.ResponseWriter, r *http.Request, sui *UserInfoStudent) {
	app.logRequestStart(r, "handleStu", slog.Int64("student_id", sui.ID))
	if r.Method != http.MethodGet {
		app.respondHTTPError(r, w, http.StatusMethodNotAllowed, "Method Not Alloewd", nil, slog.Int64("student_id", sui.ID))
		return
	}

	if _, err := fmt.Fprint(w, `Hi! You have logged on as a student (see info below) but there's
no student UI yet (poke Henry!). In the future there will be a JS SPA
over here. Note that all auth-related things are already done; you can
use the network inspector to check cookie status.

`); err != nil {
		app.logError(r, "failed writing student placeholder response", slog.Any("error", err))
		return
	}

	if err := json.NewEncoder(w).Encode(sui); err != nil {
		app.logError(r, "failed encoding student info", slog.Any("error", err))
	}
}
