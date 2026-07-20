package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sr.ht/~runxiyu/cca/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationDatabaseURLEnv = "CCA_INTEGRATION_DATABASE_URL"

var fixedIntegrationPeriods = []string{
	"Monday CCA 1",
	"Monday CCA 2",
	"Monday CCA 3",
	"Monday CCA 4",
	"Tuesday CCA 1",
	"Tuesday CCA 2",
	"Tuesday CCA 3",
	"Tuesday CCA 4",
	"Wednesday CCA 1",
	"Wednesday CCA 2",
	"Wednesday CCA 3",
	"Wednesday CCA 4",
	"Thursday CCA 1",
	"Thursday CCA 2",
	"Thursday CCA 3",
	"Thursday CCA 4",
}

type selectionRaceRequest struct {
	studentID int64
	courseID  string
	periodID  string
}

type selectionRaceResult struct {
	backendPID uint32
	err        error
}

func TestPostgresV1ToV2MigrationIntegration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv(integrationDatabaseURLEnv))
	if databaseURL == "" {
		t.Skipf("set %s to run PostgreSQL integration tests", integrationDatabaseURLEnv)
	}

	t.Run("compound aliases expand and data is preserved", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pool := newEmptyIsolatedIntegrationPool(t, databaseURL)
		loadIntegrationSQLFile(t, ctx, pool, "migrations/testdata/schema_v1.sql")

		if _, err := pool.Exec(ctx, `
			INSERT INTO grades (grade, enabled, max_own_choices)
			VALUES ('Migration Grade', TRUE, 10);
			INSERT INTO categories (id) VALUES ('Migration Category');
			INSERT INTO periods (id) VALUES
				('MW1'), ('TT3'), ('Mon 4'),
				('Monday/Wednesday CCA 2'),
				('Tuesday/Thursday CCA 4'),
				('Unused legacy label');
			INSERT INTO students (id, name, grade, legal_sex) VALUES
				(9101, 'Migration One', 'Migration Grade', 'X'),
				(9102, 'Migration Two', 'Migration Grade', 'X'),
				(9103, 'Migration Three', 'Migration Grade', 'X'),
				(9104, 'Migration Four', 'Migration Grade', 'X'),
				(9105, 'Migration Five', 'Migration Grade', 'X');
			INSERT INTO courses
				(id, name, description, period, max_students, membership, teacher, location, category_id)
			VALUES
				('M-MW1', 'MW1', '', 'MW1', 10, 'free', 'Teacher', 'Room', 'Migration Category'),
				('M-TT3', 'TT3', '', 'TT3', 10, 'free', 'Teacher', 'Room', 'Migration Category'),
				('M-MON4', 'Mon4', '', 'Mon 4', 10, 'free', 'Teacher', 'Room', 'Migration Category'),
				('M-FULL-MW2', 'Full MW2', '', 'Monday/Wednesday CCA 2', 10, 'free', 'Teacher', 'Room', 'Migration Category'),
				('M-FULL-TT4', 'Full TT4', '', 'Tuesday/Thursday CCA 4', 10, 'free', 'Teacher', 'Room', 'Migration Category');
			INSERT INTO choices (student_id, course_id, period, selection_type) VALUES
				(9101, 'M-MW1', 'MW1', 'normal'),
				(9102, 'M-TT3', 'TT3', 'normal'),
				(9103, 'M-MON4', 'Mon 4', 'normal'),
				(9104, 'M-FULL-MW2', 'Monday/Wednesday CCA 2', 'normal'),
				(9105, 'M-FULL-TT4', 'Tuesday/Thursday CCA 4', 'normal');
		`); err != nil {
			t.Fatalf("seed version 1 migration fixture: %v", err)
		}

		loadIntegrationSQLFile(t, ctx, pool, "migrations/002_multi_periods.sql")

		var version int64
		if err := pool.QueryRow(ctx, "SELECT version FROM schema_version").Scan(&version); err != nil {
			t.Fatalf("read migrated schema version: %v", err)
		}
		if version != 2 {
			t.Fatalf("schema version = %d, want 2", version)
		}

		assertMigratedCoursePeriods(t, ctx, pool, "M-MW1", []string{"Monday CCA 1", "Wednesday CCA 1"})
		assertMigratedCoursePeriods(t, ctx, pool, "M-TT3", []string{"Tuesday CCA 3", "Thursday CCA 3"})
		assertMigratedCoursePeriods(t, ctx, pool, "M-MON4", []string{"Monday CCA 4"})
		assertMigratedCoursePeriods(t, ctx, pool, "M-FULL-MW2", []string{"Monday CCA 2", "Wednesday CCA 2"})
		assertMigratedCoursePeriods(t, ctx, pool, "M-FULL-TT4", []string{"Tuesday CCA 4", "Thursday CCA 4"})

		var periodCount, choiceCount, unusedCount int
		if err := pool.QueryRow(ctx, `
			SELECT
				(SELECT count(*) FROM periods),
				(SELECT count(*) FROM choices),
				(SELECT count(*) FROM periods WHERE id = 'Unused legacy label')
		`).Scan(&periodCount, &choiceCount, &unusedCount); err != nil {
			t.Fatalf("inspect migrated data: %v", err)
		}
		if periodCount != 16 || choiceCount != 5 || unusedCount != 0 {
			t.Fatalf("migrated counts periods=%d choices=%d unused=%d, want 16, 5, 0", periodCount, choiceCount, unusedCount)
		}

		rows, err := pool.Query(ctx, `
			SELECT course_id, period_id
			FROM choices
			ORDER BY course_id
		`)
		if err != nil {
			t.Fatalf("inspect migrated choice slots: %v", err)
		}
		migratedChoiceSlots, err := pgx.CollectRows(rows, pgx.RowToMap)
		if err != nil {
			t.Fatalf("collect migrated choice slots: %v", err)
		}
		wantChoiceSlots := map[string]string{
			"M-FULL-MW2": "Monday CCA 2",
			"M-FULL-TT4": "Tuesday CCA 4",
			"M-MON4":     "Monday CCA 4",
			"M-MW1":      "Monday CCA 1",
			"M-TT3":      "Tuesday CCA 3",
		}
		for _, row := range migratedChoiceSlots {
			courseID := row["course_id"].(string)
			periodID := row["period_id"].(string)
			if wantChoiceSlots[courseID] != periodID {
				t.Fatalf("migrated choice %s period = %q, want %q", courseID, periodID, wantChoiceSlots[courseID])
			}
		}

		var batchFunctionExists bool
		if err := pool.QueryRow(ctx, `
			SELECT to_regprocedure('new_selections_batch(bigint[],text[],text[],selection_type[])') IS NOT NULL
		`).Scan(&batchFunctionExists); err != nil {
			t.Fatalf("inspect migrated batch function: %v", err)
		}
		if !batchFunctionExists {
			t.Fatal("new_selections_batch was not installed by migration 002")
		}
	})

	t.Run("unsupported occupied period rolls back atomically", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool := newEmptyIsolatedIntegrationPool(t, databaseURL)
		loadIntegrationSQLFile(t, ctx, pool, "migrations/testdata/schema_v1.sql")
		seedV1MigrationConflict(t, ctx, pool, false)

		err := execIntegrationSQLFile(t, ctx, pool, "migrations/002_multi_periods.sql")
		assertPGConstraint(t, err, "23514", "periods_unsupported")
		assertV1MigrationRollback(t, ctx, pool, 1)
	})

	t.Run("alias expansion conflict rolls back atomically", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool := newEmptyIsolatedIntegrationPool(t, databaseURL)
		loadIntegrationSQLFile(t, ctx, pool, "migrations/testdata/schema_v1.sql")
		seedV1MigrationConflict(t, ctx, pool, true)

		err := execIntegrationSQLFile(t, ctx, pool, "migrations/002_multi_periods.sql")
		assertPGConstraint(t, err, "23514", "choices_period_conflict")
		assertV1MigrationRollback(t, ctx, pool, 2)
	})
}

func TestPostgresConcurrencyIntegration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv(integrationDatabaseURLEnv))
	if databaseURL == "" {
		t.Skipf("set %s to run PostgreSQL integration tests", integrationDatabaseURLEnv)
	}

	pool := newIsolatedIntegrationPool(t, databaseURL)
	var schemaVersion int64
	if err := pool.QueryRow(context.Background(), "SELECT version FROM schema_version").Scan(&schemaVersion); err != nil {
		t.Fatalf("read fresh schema version: %v", err)
	}
	if schemaVersion != 2 {
		t.Fatalf("fresh schema version = %d, want 2", schemaVersion)
	}

	t.Run("one thousand students cannot overbook twenty five seats", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		const (
			gradeID     = "IT Capacity Grade"
			categoryID  = "IT Capacity Category"
			courseID    = "IT-CAPACITY"
			studentBase = int64(1_000_000)
			studentNo   = 1_000
		)

		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, 16)
		seedCourses(t, ctx, pool, categoryID, 25, []string{courseID}, []string{"Monday CCA 1"})
		seedStudents(t, ctx, pool, studentBase, studentNo, gradeID, "Capacity")

		requests := make([]selectionRaceRequest, studentNo)
		for i := range requests {
			requests[i] = selectionRaceRequest{
				studentID: studentBase + int64(i) + 1,
				courseID:  courseID,
				periodID:  "Monday CCA 1",
			}
		}

		results := raceSelections(ctx, pool, requests)
		assertSelectionRace(t, results, 25, "choices_capacity")
		assertMultipleBackends(t, results)

		var persisted int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM choices WHERE course_id = $1",
			courseID,
		).Scan(&persisted); err != nil {
			t.Fatalf("count capacity choices: %v", err)
		}
		if persisted != 25 {
			t.Fatalf("persisted choices = %d, want 25", persisted)
		}
	})

	t.Run("one student can win only one overlapping course", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		const (
			gradeID    = "IT Overlap Grade"
			categoryID = "IT Overlap Category"
			studentID  = int64(2_000_001)
			courseNo   = 64
		)

		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, courseNo)
		seedStudents(t, ctx, pool, studentID-1, 1, gradeID, "Overlap")

		courseIDs := make([]string, courseNo)
		periodIDs := make([]string, courseNo)
		requests := make([]selectionRaceRequest, courseNo)
		for i := range courseNo {
			courseIDs[i] = fmt.Sprintf("IT-OVERLAP-%03d", i+1)
			periodIDs[i] = "Tuesday CCA 1"
			requests[i] = selectionRaceRequest{studentID: studentID, courseID: courseIDs[i], periodID: periodIDs[i]}
		}
		seedCourses(t, ctx, pool, categoryID, courseNo, courseIDs, periodIDs)

		results := raceSelections(ctx, pool, requests)
		assertSelectionRace(t, results, 1, "choices_period_conflict")

		var persisted, busiestPeriod int
		if err := pool.QueryRow(ctx, `
			SELECT
				count(DISTINCT ch.course_id)::int,
				COALESCE(max(period_count), 0)::int
			FROM choices ch
			LEFT JOIN (
				SELECT cp.period_id, count(*) AS period_count
				FROM choices selected
				JOIN course_periods cp ON cp.course_id = selected.course_id
				WHERE selected.student_id = $1
				GROUP BY cp.period_id
			) occupied ON TRUE
			WHERE ch.student_id = $1
		`, studentID).Scan(&persisted, &busiestPeriod); err != nil {
			t.Fatalf("inspect overlapping choices: %v", err)
		}
		if persisted != 1 || busiestPeriod != 1 {
			t.Fatalf("overlap state choices=%d busiest_period=%d, want 1 and 1", persisted, busiestPeriod)
		}
	})

	t.Run("normal choice limit is atomic across non overlapping slots", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		const (
			gradeID    = "IT Limit Grade"
			categoryID = "IT Limit Category"
			studentID  = int64(3_000_001)
		)

		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, 3)
		seedStudents(t, ctx, pool, studentID-1, 1, gradeID, "Limit")

		courseIDs := make([]string, len(fixedIntegrationPeriods))
		requests := make([]selectionRaceRequest, len(fixedIntegrationPeriods))
		for i := range fixedIntegrationPeriods {
			courseIDs[i] = fmt.Sprintf("IT-LIMIT-%02d", i+1)
			requests[i] = selectionRaceRequest{studentID: studentID, courseID: courseIDs[i], periodID: fixedIntegrationPeriods[i]}
		}
		seedCourses(t, ctx, pool, categoryID, 100, courseIDs, fixedIntegrationPeriods)

		results := raceSelections(ctx, pool, requests)
		assertSelectionRace(t, results, 3, "choices_max_own")

		var persisted int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM choices
			WHERE student_id = $1 AND selection_type = 'normal'
		`, studentID).Scan(&persisted); err != nil {
			t.Fatalf("count limited choices: %v", err)
		}
		if persisted != 3 {
			t.Fatalf("persisted normal choices = %d, want 3", persisted)
		}
	})

	t.Run("multi-slot course reserves only the chosen slot", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		const (
			gradeID       = "IT Slot Choice Grade"
			categoryID    = "IT Slot Choice Category"
			studentID     = int64(3_500_001)
			multiCourseID = "IT-SLOT-MULTI"
			mondayCourse  = "IT-SLOT-MONDAY"
			tuesdayCourse = "IT-SLOT-TUESDAY"
		)
		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, 4)
		seedStudents(t, ctx, pool, studentID-1, 1, gradeID, "Slot Choice")
		seedCourses(
			t,
			ctx,
			pool,
			categoryID,
			20,
			[]string{multiCourseID, mondayCourse, tuesdayCourse},
			[]string{"Monday CCA 1", "Monday CCA 1", "Tuesday CCA 1"},
		)
		if _, err := pool.Exec(ctx, `
			INSERT INTO course_periods (course_id, period_id)
			VALUES ($1, 'Tuesday CCA 1')
		`, multiCourseID); err != nil {
			t.Fatalf("add alternative course slot: %v", err)
		}

		if _, err := pool.Exec(ctx,
			"SELECT new_selection($1, $2, $3, 'normal'::selection_type)",
			studentID,
			multiCourseID,
			"Tuesday CCA 1",
		); err != nil {
			t.Fatalf("select Tuesday alternative: %v", err)
		}
		if _, err := pool.Exec(ctx,
			"SELECT new_selection($1, $2, $3, 'normal'::selection_type)",
			studentID,
			mondayCourse,
			"Monday CCA 1",
		); err != nil {
			t.Fatalf("Monday slot should remain available: %v", err)
		}

		_, err := pool.Exec(ctx,
			"SELECT new_selection($1, $2, $3, 'force'::selection_type)",
			studentID,
			tuesdayCourse,
			"Tuesday CCA 1",
		)
		assertPGConstraint(t, err, "23514", "choices_period_conflict")

		var chosenPeriod string
		if err := pool.QueryRow(ctx, `
			SELECT period_id
			FROM choices
			WHERE student_id = $1 AND course_id = $2
		`, studentID, multiCourseID).Scan(&chosenPeriod); err != nil {
			t.Fatalf("read chosen alternative: %v", err)
		}
		if chosenPeriod != "Tuesday CCA 1" {
			t.Fatalf("chosen period = %q, want Tuesday CCA 1", chosenPeriod)
		}
	})

	t.Run("fixed periods are complete ordered and immutable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		rows, err := pool.Query(ctx, "SELECT id FROM periods ORDER BY ordinal")
		if err != nil {
			t.Fatalf("query fixed periods: %v", err)
		}
		periods, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("collect fixed periods: %v", err)
		}
		if !slices.Equal(periods, fixedIntegrationPeriods) {
			t.Fatalf("periods = %#v, want %#v", periods, fixedIntegrationPeriods)
		}
		for _, period := range periods {
			if strings.Contains(strings.ToLower(period), "friday") {
				t.Fatalf("unexpected Friday period %q", period)
			}
		}

		mutations := []struct {
			name string
			sql  string
		}{
			{name: "insert", sql: "INSERT INTO periods (id, ordinal) VALUES ('Friday CCA 1', 17)"},
			{name: "update", sql: "UPDATE periods SET id = id WHERE ordinal = 1"},
			{name: "delete", sql: "DELETE FROM periods WHERE FALSE"},
			{name: "truncate", sql: "TRUNCATE periods CASCADE"},
		}
		for _, mutation := range mutations {
			t.Run(mutation.name, func(t *testing.T) {
				_, err := pool.Exec(ctx, mutation.sql)
				assertPGConstraint(t, err, "23514", "periods_fixed")
			})
		}
	})

	t.Run("course period invariant is deferred until commit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		const (
			categoryID        = "IT Deferred Category"
			missingCourseID   = "IT-NO-PERIOD"
			scheduledCourseID = "IT-DEFERRED-VALID"
		)
		if _, err := pool.Exec(ctx, "INSERT INTO categories (id) VALUES ($1)", categoryID); err != nil {
			t.Fatalf("insert deferred-test category: %v", err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin missing-period transaction: %v", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO courses
				(id, name, description, max_students, membership, teacher, location, category_id)
			VALUES ($1, $1, '', 10, 'free', 'Teacher', 'Room', $2)
		`, missingCourseID, categoryID)
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("course insert failed before deferred constraint was checked: %v", err)
		}
		assertPGConstraint(t, tx.Commit(ctx), "23514", "course_requires_period")

		var missingRows int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM courses WHERE id = $1",
			missingCourseID,
		).Scan(&missingRows); err != nil {
			t.Fatalf("check rolled-back missing-period course: %v", err)
		}
		if missingRows != 0 {
			t.Fatalf("missing-period course persisted after failed commit")
		}

		seedCourses(t, ctx, pool, categoryID, 10,
			[]string{scheduledCourseID},
			[]string{"Thursday CCA 4"},
		)

		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin last-period deletion: %v", err)
		}
		if _, err := tx.Exec(ctx,
			"DELETE FROM course_periods WHERE course_id = $1",
			scheduledCourseID,
		); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("last-period delete failed before deferred constraint was checked: %v", err)
		}
		assertPGConstraint(t, tx.Commit(ctx), "23514", "course_requires_period")

		var remainingPeriods int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM course_periods WHERE course_id = $1",
			scheduledCourseID,
		).Scan(&remainingPeriods); err != nil {
			t.Fatalf("check restored course period: %v", err)
		}
		if remainingPeriods != 1 {
			t.Fatalf("course periods after failed delete commit = %d, want 1", remainingPeriods)
		}
	})

	t.Run("selection batch validates shape and inserts atomically", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		const (
			gradeID     = "IT Batch Grade"
			categoryID  = "IT Batch Category"
			studentBase = int64(4_000_000)
		)
		courseIDs := []string{"IT-BATCH-1", "IT-BATCH-2"}
		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, 2)
		seedStudents(t, ctx, pool, studentBase, 2, gradeID, "Batch")
		seedCourses(t, ctx, pool, categoryID, 10, courseIDs,
			[]string{"Monday CCA 2", "Tuesday CCA 2"},
		)

		_, err := pool.Exec(ctx, `
			SELECT new_selections_batch(
				ARRAY[$1::bigint, $2::bigint],
				ARRAY[$3::text],
				ARRAY['Monday CCA 2'::text, 'Tuesday CCA 2'::text],
				ARRAY['normal'::selection_type, 'normal'::selection_type]
			)
		`, studentBase+1, studentBase+2, courseIDs[0])
		assertPGConstraint(t, err, "22023", "selection_batch_shape")

		var inserted int64
		if err := pool.QueryRow(ctx, `
			SELECT new_selections_batch(
				ARRAY[$1::bigint, $2::bigint],
				ARRAY[$3::text, $4::text],
				ARRAY['Monday CCA 2'::text, 'Tuesday CCA 2'::text],
				ARRAY['normal'::selection_type, 'normal'::selection_type]
			)
		`, studentBase+1, studentBase+2, courseIDs[0], courseIDs[1]).Scan(&inserted); err != nil {
			t.Fatalf("insert selection batch: %v", err)
		}
		if inserted != 2 {
			t.Fatalf("batch inserted %d rows, want 2", inserted)
		}

		var persisted int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM choices
			WHERE student_id = ANY($1::bigint[])
		`, []int64{studentBase + 1, studentBase + 2}).Scan(&persisted); err != nil {
			t.Fatalf("count batch choices: %v", err)
		}
		if persisted != 2 {
			t.Fatalf("persisted batch choices = %d, want 2", persisted)
		}
	})

	t.Run("student catalogue and requirement progress are evaluated in SQL", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		const (
			gradeID     = "IT Catalog Grade"
			categoryID  = "IT Catalog Category"
			studentID   = int64(5_000_001)
			selectedID  = "IT-CATALOG-SELECTED"
			conflictID  = "IT-CATALOG-CONFLICT"
			availableID = "IT-CATALOG-AVAILABLE"
			fullID      = "IT-CATALOG-FULL"
		)
		seedGradeAndCategory(t, ctx, pool, gradeID, categoryID, 2)
		seedStudents(t, ctx, pool, studentID-1, 1, gradeID, "Catalog")
		seedCourses(
			t,
			ctx,
			pool,
			categoryID,
			10,
			[]string{selectedID, conflictID, availableID},
			[]string{"Monday CCA 1", "Monday CCA 1", "Tuesday CCA 1"},
		)
		seedCourses(t, ctx, pool, categoryID, 0, []string{fullID}, []string{"Wednesday CCA 1"})

		if _, err := pool.Exec(ctx,
			"SELECT new_selection($1, $2, $3, 'normal'::selection_type)",
			studentID,
			selectedID,
			"Monday CCA 1",
		); err != nil {
			t.Fatalf("seed selected catalogue course: %v", err)
		}
		var requirementID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO grade_requirement_groups (grade, min_count)
			VALUES ($1, 2)
			RETURNING id
		`, gradeID).Scan(&requirementID); err != nil {
			t.Fatalf("insert catalogue requirement: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO grade_requirement_group_categories (req_group_id, category_id)
			VALUES ($1, $2)
		`, requirementID, categoryID); err != nil {
			t.Fatalf("insert catalogue requirement category: %v", err)
		}

		queries := db.New(pool)
		student := UserInfoStudent(db.GetStudentBySessionRow{
			ID:       studentID,
			Name:     "Catalog 1",
			Grade:    gradeID,
			LegalSex: db.LegalSexX,
		})
		views, err := listStudentCourseViewsWithQueries(ctx, queries, &student)
		if err != nil {
			t.Fatalf("get SQL student catalogue: %v", err)
		}
		byID := make(map[string]CourseView, len(views))
		for _, view := range views {
			byID[view.ID] = view
		}
		if selected := byID[selectedID]; !selected.Selected || !selected.Available || !selected.Removable {
			t.Fatalf("selected catalogue state = %#v, want selected, available, removable", selected)
		}
		if available := byID[availableID]; !available.Available || len(available.BlockReasons) != 0 {
			t.Fatalf("available catalogue state = %#v, want no block reasons", available)
		}
		if conflict := byID[conflictID]; conflict.Available || !containsCourseBlockReason(conflict, "schedule_conflict") {
			t.Fatalf("conflict catalogue state = %#v, want schedule conflict", conflict)
		}
		if full := byID[fullID]; full.Available || !containsCourseBlockReason(full, "course_full") {
			t.Fatalf("full catalogue state = %#v, want course full", full)
		}

		progress, err := queries.GetStudentRequirementProgress(ctx, studentID)
		if err != nil {
			t.Fatalf("get SQL requirement progress: %v", err)
		}
		if len(progress) != 1 || progress[0].CurrentCount != 1 || progress[0].MinCount != 2 {
			t.Fatalf("requirement progress = %#v, want 1 of 2", progress)
		}
	})
}

func containsCourseBlockReason(course CourseView, code string) bool {
	for _, reason := range course.BlockReasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func assertMigratedCoursePeriods(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	courseID string,
	want []string,
) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT cp.period_id
		FROM course_periods cp
		JOIN periods p ON p.id = cp.period_id
		WHERE cp.course_id = $1
		ORDER BY p.ordinal
	`, courseID)
	if err != nil {
		t.Fatalf("query migrated periods for %s: %v", courseID, err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect migrated periods for %s: %v", courseID, err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("periods for %s = %#v, want %#v", courseID, got, want)
	}
}

func seedV1MigrationConflict(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	expansionConflict bool,
) {
	t.Helper()
	var sql string
	if expansionConflict {
		sql = `
			INSERT INTO periods (id) VALUES ('MW1'), ('Mon 1');
			INSERT INTO students (id, name, grade, legal_sex)
			VALUES (9201, 'Conflict Student', 'Migration Rollback Grade', 'X');
			INSERT INTO courses
				(id, name, description, period, max_students, membership, teacher, location, category_id)
			VALUES
				('ROLLBACK-MW', 'MW', '', 'MW1', 10, 'free', 'Teacher', 'Room', 'Migration Rollback Category'),
				('ROLLBACK-MON', 'Mon', '', 'Mon 1', 10, 'free', 'Teacher', 'Room', 'Migration Rollback Category');
			INSERT INTO choices (student_id, course_id, period, selection_type) VALUES
				(9201, 'ROLLBACK-MW', 'MW1', 'normal'),
				(9201, 'ROLLBACK-MON', 'Mon 1', 'normal');
		`
	} else {
		sql = `
			INSERT INTO periods (id) VALUES ('Fri 1');
			INSERT INTO students (id, name, grade, legal_sex)
			VALUES (9201, 'Unsupported Student', 'Migration Rollback Grade', 'X');
			INSERT INTO courses
				(id, name, description, period, max_students, membership, teacher, location, category_id)
			VALUES
				('ROLLBACK-FRI', 'Friday', '', 'Fri 1', 10, 'free', 'Teacher', 'Room', 'Migration Rollback Category');
			INSERT INTO choices (student_id, course_id, period, selection_type)
			VALUES (9201, 'ROLLBACK-FRI', 'Fri 1', 'normal');
		`
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO grades (grade, enabled, max_own_choices)
		VALUES ('Migration Rollback Grade', TRUE, 10);
		INSERT INTO categories (id) VALUES ('Migration Rollback Category');
	`+sql); err != nil {
		t.Fatalf("seed version 1 rollback case: %v", err)
	}
}

func assertV1MigrationRollback(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	wantChoices int,
) {
	t.Helper()
	var version int64
	var choices int
	var coursePeriodsMissing, legacyPeriodColumnPresent bool
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT version FROM schema_version),
			(SELECT count(*) FROM choices),
			to_regclass('course_periods') IS NULL,
			EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = current_schema()
					AND table_name = 'courses'
					AND column_name = 'period'
			)
	`).Scan(&version, &choices, &coursePeriodsMissing, &legacyPeriodColumnPresent); err != nil {
		t.Fatalf("inspect version 1 rollback: %v", err)
	}
	if version != 1 || choices != wantChoices || !coursePeriodsMissing || !legacyPeriodColumnPresent {
		t.Fatalf(
			"rollback state version=%d choices=%d course_periods_missing=%v legacy_period=%v; want 1, %d, true, true",
			version,
			choices,
			coursePeriodsMissing,
			legacyPeriodColumnPresent,
			wantChoices,
		)
	}
}

func newIsolatedIntegrationPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool := newEmptyIsolatedIntegrationPool(t, databaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	loadIntegrationSQLFile(t, ctx, pool, "schema.sql")
	return pool
}

func newEmptyIsolatedIntegrationPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", integrationDatabaseURLEnv, err)
	}
	baseConfig.MaxConns = 2
	basePool, err := pgxpool.NewWithConfig(ctx, baseConfig)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := basePool.Ping(ctx); err != nil {
		basePool.Close()
		t.Fatalf("ping integration database: %v", err)
	}

	schemaName := fmt.Sprintf("cca_it_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := basePool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		basePool.Close()
		t.Fatalf("create isolated schema: %v", err)
	}

	testConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = basePool.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE")
		basePool.Close()
		t.Fatalf("parse isolated pool config: %v", err)
	}
	testConfig.MaxConns = 24
	testConfig.MinConns = 8
	if testConfig.ConnConfig.RuntimeParams == nil {
		testConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	testConfig.ConnConfig.RuntimeParams["search_path"] = quotedSchema

	pool, err := pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		_, _ = basePool.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE")
		basePool.Close()
		t.Fatalf("open isolated integration pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := basePool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop isolated schema %s: %v", schemaName, err)
		}
		basePool.Close()
	})

	// Hold several connections simultaneously once so later barriers exercise
	// real database concurrency rather than only goroutine scheduling.
	warmConnections := make([]*pgxpool.Conn, 0, 8)
	for range 8 {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			for _, warm := range warmConnections {
				warm.Release()
			}
			t.Fatalf("prewarm integration pool: %v", err)
		}
		warmConnections = append(warmConnections, conn)
	}
	for _, warm := range warmConnections {
		warm.Release()
	}

	return pool
}

func execIntegrationSQLFile(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	path string,
) error {
	t.Helper()
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection for %s: %v", path, err)
	}
	defer conn.Release()
	_, err = conn.Conn().PgConn().Exec(ctx, string(sqlBytes)).ReadAll()
	return err
}

func loadIntegrationSQLFile(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	path string,
) {
	t.Helper()
	if err := execIntegrationSQLFile(t, ctx, pool, path); err != nil {
		t.Fatalf("execute %s: %v", path, err)
	}
}

func seedGradeAndCategory(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	gradeID string,
	categoryID string,
	maxOwnChoices int,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO grades (grade, enabled, max_own_choices)
		VALUES ($1, TRUE, $2)
	`, gradeID, maxOwnChoices); err != nil {
		t.Fatalf("insert grade %q: %v", gradeID, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO categories (id) VALUES ($1)", categoryID); err != nil {
		t.Fatalf("insert category %q: %v", categoryID, err)
	}
}

func seedStudents(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	studentBase int64,
	studentNo int,
	gradeID string,
	namePrefix string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO students (id, name, grade, legal_sex)
		SELECT
			$1::bigint + generated.id::bigint,
			$2::text || ' ' || generated.id::text,
			$3,
			'X'::legal_sex
		FROM generate_series(1, $4::integer) generated(id)
	`, studentBase, namePrefix, gradeID, studentNo); err != nil {
		t.Fatalf("insert %d students for grade %q: %v", studentNo, gradeID, err)
	}
}

func seedCourses(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	categoryID string,
	maxStudents int,
	courseIDs []string,
	periodIDs []string,
) {
	t.Helper()
	if len(courseIDs) != len(periodIDs) {
		t.Fatalf("seed course IDs and period IDs have different lengths")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin course seed: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO courses
			(id, name, description, max_students, membership, teacher, location, category_id)
		SELECT
			course.id,
			course.id,
			'',
			$2,
			'free',
			'Integration Teacher',
			'Integration Room',
			$1
		FROM unnest($3::text[]) course(id)
	`, categoryID, maxStudents, courseIDs); err != nil {
		t.Fatalf("insert integration courses: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO course_periods (course_id, period_id)
		SELECT requested.course_id, requested.period_id
		FROM unnest($1::text[], $2::text[]) requested(course_id, period_id)
	`, courseIDs, periodIDs); err != nil {
		t.Fatalf("insert integration course periods: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit course seed: %v", err)
	}
}

func raceSelections(
	ctx context.Context,
	pool *pgxpool.Pool,
	requests []selectionRaceRequest,
) []selectionRaceResult {
	start := make(chan struct{})
	results := make(chan selectionRaceResult, len(requests))
	var ready sync.WaitGroup
	ready.Add(len(requests))

	for _, request := range requests {
		go func() {
			ready.Done()
			<-start

			conn, err := pool.Acquire(ctx)
			if err != nil {
				results <- selectionRaceResult{err: err}
				return
			}
			pid := conn.Conn().PgConn().PID()
			_, err = conn.Exec(ctx,
				"SELECT new_selection($1, $2, $3, 'normal'::selection_type)",
				request.studentID,
				request.courseID,
				request.periodID,
			)
			conn.Release()
			results <- selectionRaceResult{backendPID: pid, err: err}
		}()
	}

	ready.Wait()
	close(start)

	collected := make([]selectionRaceResult, 0, len(requests))
	for range requests {
		collected = append(collected, <-results)
	}
	return collected
}

func assertSelectionRace(
	t *testing.T,
	results []selectionRaceResult,
	wantSuccess int,
	wantFailureConstraint string,
) {
	t.Helper()

	success := 0
	expectedFailures := 0
	unexpected := make(map[string]int)
	for _, result := range results {
		if result.err == nil {
			success++
			continue
		}
		var pgErr *pgconn.PgError
		if errors.As(result.err, &pgErr) &&
			pgErr.Code == "23514" &&
			pgErr.ConstraintName == wantFailureConstraint {
			expectedFailures++
			continue
		}
		unexpected[describeIntegrationError(result.err)]++
	}

	if success != wantSuccess || expectedFailures != len(results)-wantSuccess || len(unexpected) != 0 {
		t.Fatalf(
			"race outcome success=%d expected_failures=%d unexpected=%v; want success=%d and constraint %q",
			success,
			expectedFailures,
			unexpected,
			wantSuccess,
			wantFailureConstraint,
		)
	}
}

func assertMultipleBackends(t *testing.T, results []selectionRaceResult) {
	t.Helper()
	backendPIDs := make(map[uint32]struct{})
	for _, result := range results {
		if result.backendPID != 0 {
			backendPIDs[result.backendPID] = struct{}{}
		}
	}
	if len(backendPIDs) < 2 {
		t.Fatalf("selection race used %d PostgreSQL backend, want at least 2", len(backendPIDs))
	}
}

func assertPGConstraint(t *testing.T, err error, code string, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("operation succeeded, want SQLSTATE %s constraint %q", code, constraint)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("operation error = %T %v, want PostgreSQL constraint error", err, err)
	}
	if pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf(
			"operation SQLSTATE=%s constraint=%q, want SQLSTATE=%s constraint=%q",
			pgErr.Code,
			pgErr.ConstraintName,
			code,
			constraint,
		)
	}
}

func describeIntegrationError(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Sprintf("postgres code=%s constraint=%s", pgErr.Code, pgErr.ConstraintName)
	}
	return fmt.Sprintf("%T: %v", err, err)
}
