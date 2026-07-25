---- Periods

-- name: GetPeriods :many
SELECT id
FROM periods
ORDER BY ordinal;

-- name: GetCoursePeriodAssignments :many
SELECT cp.course_id, cp.period_id
FROM course_periods cp
JOIN periods p ON p.id = cp.period_id
ORDER BY cp.course_id, p.ordinal;

-- name: GetCoursePeriodsByCourse :many
SELECT cp.period_id
FROM course_periods cp
JOIN periods p ON p.id = cp.period_id
WHERE cp.course_id = $1
ORDER BY p.ordinal;

-- name: AddCoursePeriod :exec
INSERT INTO course_periods (course_id, period_id)
VALUES ($1, $2)
ON CONFLICT (course_id, period_id) DO NOTHING;

-- name: DeleteCoursePeriodsExcept :exec
DELETE FROM course_periods
WHERE course_id = sqlc.arg(course_id)
	AND NOT (period_id = ANY(sqlc.arg(period_ids)::text[]));

-- name: DeleteCoursePeriods :exec
DELETE FROM course_periods
WHERE course_id = $1;
