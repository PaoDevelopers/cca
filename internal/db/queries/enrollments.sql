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

-- Import rows set aside to decide later; see 0008. A draft holds only
-- seats not taken: one enrolled since, by the student or by hand, is
-- settled, and resuming it would overwrite that enrollment's policy.
-- name: GetPendingEnrollments :many
SELECT student_id, course_id, student_droppable, counts_toward_budget, source
FROM pending_enrollments p
WHERE NOT EXISTS (SELECT 1 FROM enrollments e
	WHERE e.student_id = p.student_id AND e.course_id = p.course_id)
ORDER BY source, position;

-- The settled rows GetPendingEnrollments hides, cleared by each import.
-- name: DeleteSettledPending :exec
DELETE FROM pending_enrollments p
USING enrollments e
WHERE e.student_id = p.student_id AND e.course_id = p.course_id;

-- Clears the pending rows an import names: the upload decides each one,
-- whether by placing it, leaving it out, or setting it aside again. A
-- file closed undecided (only_source) clears only its own draft's.
-- name: DeletePendingEnrollments :exec
DELETE FROM pending_enrollments p
USING unnest(sqlc.arg(course_ids)::TEXT[], sqlc.arg(student_ids)::TEXT[])
	AS u (course_id, student_id)
WHERE p.course_id = u.course_id AND p.student_id = u.student_id
	AND (NOT sqlc.arg(only_source)::BOOLEAN OR p.source = sqlc.arg(source)::TEXT);

-- Rows naming a student or course that no longer exists are skipped,
-- not refused: one deleted since the check has nothing to wait for; so
-- are seats already taken, which a draft does not hold (above). A
-- row another draft holds moves here only when the administrator chose
-- to set it aside, not when a file was merely closed (only_source).
-- name: InsertPendingEnrollments :execrows
INSERT INTO pending_enrollments
	(course_id, student_id, student_droppable, counts_toward_budget, source, position)
SELECT u.course_id, u.student_id, u.droppable, u.budgeted,
	sqlc.arg(source)::TEXT, u.position
FROM unnest(sqlc.arg(course_ids)::TEXT[], sqlc.arg(student_ids)::TEXT[],
	sqlc.arg(droppable)::BOOLEAN[], sqlc.arg(budgeted)::BOOLEAN[])
	WITH ORDINALITY AS u (course_id, student_id, droppable, budgeted, position)
WHERE EXISTS (SELECT 1 FROM students s WHERE s.id = u.student_id)
	AND EXISTS (SELECT 1 FROM courses c WHERE c.id = u.course_id)
	AND NOT EXISTS (SELECT 1 FROM enrollments e
		WHERE e.student_id = u.student_id AND e.course_id = u.course_id)
ON CONFLICT (student_id, course_id) DO UPDATE SET
	student_droppable = excluded.student_droppable,
	counts_toward_budget = excluded.counts_toward_budget,
	source = excluded.source,
	position = excluded.position
WHERE NOT sqlc.arg(only_source)::BOOLEAN
	OR pending_enrollments.source = excluded.source;

-- One file's draft, thrown away.
-- name: DeletePendingSource :exec
DELETE FROM pending_enrollments WHERE source = $1;
