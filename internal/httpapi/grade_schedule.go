package httpapi

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const gradeSelectionSchedulePollInterval = 15 * time.Second

type gradeSelectionScheduleView struct {
	BatchID  int64      `json:"batch_id"`
	GradeIDs []string   `json:"grade_ids"`
	OpensAt  time.Time  `json:"opens_at"`
	ClosesAt *time.Time `json:"closes_at,omitempty"`
	Opened   bool       `json:"opened"`
}

func normalizeGradeIDs(values []string) []string {
	grades := make([]string, 0, len(values))
	for _, value := range values {
		if grade := strings.TrimSpace(value); grade != "" {
			grades = append(grades, grade)
		}
	}
	slices.Sort(grades)
	return slices.Compact(grades)
}

type gradeScheduleQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func listGradeSelectionSchedulesWithQuerier(ctx context.Context, querier gradeScheduleQuerier) ([]gradeSelectionScheduleView, error) {
	rows, err := querier.Query(ctx, `
		SELECT
			batch_id,
			array_agg(grade ORDER BY grade),
			min(opens_at),
			min(closes_at),
			bool_and(opened)
		FROM grade_selection_schedules
		GROUP BY batch_id
		ORDER BY
			CASE
				WHEN bool_and(opened) THEN min(closes_at)
				ELSE min(opens_at)
			END,
			batch_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := make([]gradeSelectionScheduleView, 0)
	for rows.Next() {
		var schedule gradeSelectionScheduleView
		var closesAt pgtype.Timestamptz
		if err := rows.Scan(
			&schedule.BatchID,
			&schedule.GradeIDs,
			&schedule.OpensAt,
			&closesAt,
			&schedule.Opened,
		); err != nil {
			return nil, err
		}
		if closesAt.Valid {
			value := closesAt.Time
			schedule.ClosesAt = &value
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (app *App) setGradeSelectionAccess(ctx context.Context, grades []string, enabled bool) (int64, int64, error) {
	var updated int64
	var cancelled int64
	err := app.pool.QueryRow(ctx,
		`SELECT updated_count, cancelled_schedule_count
		 FROM set_grade_selection_access($1, $2)`,
		grades,
		enabled,
	).Scan(&updated, &cancelled)
	return updated, cancelled, err
}

func (app *App) updateGradeSettings(ctx context.Context, grade string, enabled *bool, maxOwnChoices int64) (bool, error) {
	var updated bool
	err := app.pool.QueryRow(ctx,
		`SELECT update_grade_settings_with_schedule($1, $2, $3)`,
		grade,
		enabled,
		maxOwnChoices,
	).Scan(&updated)
	return updated, err
}

func (app *App) saveGradeSelectionSchedule(
	ctx context.Context,
	batchID *int64,
	grades []string,
	opensAt time.Time,
	closesAt *time.Time,
	replaceExisting bool,
) (int64, error) {
	var savedBatchID int64
	err := app.pool.QueryRow(ctx,
		`SELECT save_grade_selection_schedule($1, $2, $3, $4, $5)`,
		batchID,
		grades,
		opensAt,
		closesAt,
		replaceExisting,
	).Scan(&savedBatchID)
	return savedBatchID, err
}

func (app *App) cancelGradeSelectionSchedule(ctx context.Context, batchID int64) (int64, error) {
	var deleted int64
	err := app.pool.QueryRow(ctx,
		`SELECT cancel_grade_selection_schedule($1)`,
		batchID,
	).Scan(&deleted)
	return deleted, err
}

func (app *App) applyDueGradeSelectionSchedules(ctx context.Context, now time.Time) (int64, int64, error) {
	var processed int64
	var revision int64
	err := app.pool.QueryRow(ctx,
		`SELECT processed_count, revision
		 FROM apply_due_grade_selection_schedules($1)`,
		now,
	).Scan(&processed, &revision)
	return processed, revision, err
}

func (app *App) runGradeSelectionScheduler(ctx context.Context) {
	ticker := time.NewTicker(gradeSelectionSchedulePollInterval)
	defer ticker.Stop()

	var lastRevision int64
	haveRevision := false
	apply := func() {
		processed, revision, err := app.applyDueGradeSelectionSchedules(ctx, time.Now())
		if err != nil {
			slog.Error("apply grade selection schedules", slog.Any("error", err))
			return
		}
		if processed > 0 || (haveRevision && revision != lastRevision) {
			app.wsHub.Broadcast(WSMessage("invalidate_grades"))
		}
		lastRevision = revision
		haveRevision = true
	}

	apply()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apply()
		}
	}
}
