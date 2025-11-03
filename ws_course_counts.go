package main

import (
	"log/slog"
	"net/http"
	"strconv"
)

func (app *App) broadcastCourseCounts(r *http.Request, courseIDs []string) {
	if len(courseIDs) == 0 {
		return
	}

	dedup := make([]string, 0, len(courseIDs))
	seen := make(map[string]struct{}, len(courseIDs))
	for _, id := range courseIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		dedup = append(dedup, id)
	}

	if len(dedup) == 0 {
		return
	}

	rows, err := app.queries.GetCourseCountsByIDs(r.Context(), dedup)
	if err != nil {
		app.logError(r, logMsgAdminCoursesCountsError, slog.Any("error", err))
		return
	}

	counts := make(map[string]int64, len(dedup))
	for _, row := range rows {
		counts[row.ID] = row.CurrentStudents
	}

	for _, id := range dedup {
		count := counts[id]
		app.wsHub.Broadcast(WSMessage("course_count_update," + id + "," + strconv.FormatInt(count, 10)))
	}
}
