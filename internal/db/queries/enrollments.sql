-- name: GetEnrollments :many
SELECT * FROM v_enrollments ORDER BY student_id, course_id;

-- name: GetEnrollmentsByStudent :many
SELECT * FROM v_enrollments WHERE student_id = $1 ORDER BY course_id;

-- name: EnrollmentViolations :many
SELECT rule, code, other_course_id, period_id, detail
FROM enrollment_violations($1, $2, $3, $4);

-- name: SelfEnroll :exec
SELECT self_enroll($1, $2);

-- name: SelfDrop :exec
SELECT self_drop($1, $2);

-- name: SelfSwap :exec
SELECT self_swap($1, $2, $3);

-- name: PlaceEnrollments :exec
SELECT place_enrollments($1, $2, $3, $4, $5);

-- name: RemoveEnrollments :exec
SELECT remove_enrollments($1, $2);

-- name: StudentCourseViolations :many
SELECT course_id, rule, code, other_course_id, period_id, detail
FROM student_course_violations($1, $2);

-- An unscheduled course has no period, which the spreadsheet wants as
-- an empty cell rather than a NULL; absence is '' here as everywhere.
-- name: GetEnrollmentsExport :many
SELECT student_id, student_name, grade_id, legal_sex,
	course_id, course_name, term, COALESCE(period_id, '')::TEXT AS period_id,
	student_droppable, counts_toward_budget
FROM v_export_enrollments
ORDER BY student_id, course_id, period_sort_order NULLS FIRST, period_id;

-- name: SetEnrollmentPolicy :exec
SELECT set_enrollment_policy($1, $2, $3, $4, $5);
