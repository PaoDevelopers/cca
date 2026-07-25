package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const courseStateQueryTimeout = 2 * time.Second

func (app *App) publishCourseStates(r *http.Request, courseIDs []string) {
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

	// The mutation has already committed. Do not lose its realtime update only
	// because the caller disconnected while the response was being assembled.
	ctx, cancel := context.WithTimeout(context.Background(), courseStateQueryTimeout)
	defer cancel()
	rows, err := app.queries.GetCourseStatesByIDs(ctx, dedup)
	if err != nil {
		app.logError(r, logMsgAdminCoursesCountsError, slog.Any("error", err))
		return
	}

	states := make([]CourseStateUpdate, 0, len(rows))
	for _, row := range rows {
		states = append(states, CourseStateUpdate{
			CourseID:        row.ID,
			CurrentStudents: row.CurrentStudents,
			StateRevision:   row.StateRevision,
		})
	}
	app.wsHub.PublishCourseStates(states)
}
