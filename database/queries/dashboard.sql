---- Admin dashboard

-- name: GetAdminDashboardCounts :one
SELECT
	(SELECT COUNT(*) FROM courses)::bigint AS course_count,
	(
		SELECT COUNT(*)
		FROM courses course
		WHERE NOT EXISTS (
			SELECT 1
			FROM course_periods assignment
			WHERE assignment.course_id = course.id
		)
	)::bigint AS courses_without_timetable,
	(SELECT COUNT(*) FROM students)::bigint AS student_count;
