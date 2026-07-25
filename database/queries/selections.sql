---- Selections

-- name: GetSelections :many
SELECT
	ch.student_id,
	s.name AS student_name,
	s.grade AS student_grade,
	ch.course_id,
	c.name AS course_name,
	ch.period_id,
	ch.selection_type
FROM choices ch
JOIN students s ON s.id = ch.student_id
JOIN courses c ON c.id = ch.course_id
ORDER BY ch.student_id, ch.course_id;

-- name: GetSelectionsExport :many
SELECT
	student_id,
	student_name,
	grade,
	legal_sex,
	course_id,
	course_name,
	periods,
	selection_type
FROM v_export_selections;

-- name: NewSelection :exec
SELECT new_selection($1, $2, $3, $4);

-- name: CreateStudentSelection :one
WITH mutation AS MATERIALIZED (
	SELECT new_selection(
		sqlc.arg(student_id),
		sqlc.arg(course_id),
		sqlc.arg(period_id),
		'normal'::selection_type
	)
)
SELECT
	state.course_id,
	state.current_students,
	state.revision AS state_revision
FROM mutation
JOIN course_selection_state state
	ON state.course_id = sqlc.arg(course_id);

-- name: NewSelectionsBatch :one
SELECT new_selections_batch(
	sqlc.arg(student_ids)::bigint[],
	sqlc.arg(course_ids)::text[],
	sqlc.arg(period_ids)::text[],
	sqlc.arg(selection_types)::text[]::selection_type[]
) AS inserted_count;

-- name: UpdateSelection :execrows
UPDATE choices AS ch
SET
	course_id = sqlc.arg(new_course_id),
	period_id = sqlc.arg(new_period_id),
	selection_type = sqlc.arg(selection_type)
WHERE
	ch.student_id = sqlc.arg(student_id)
	AND ch.course_id = sqlc.arg(current_course_id);

-- name: DeleteSelection :exec
DELETE FROM choices
WHERE student_id = $1 AND course_id = $2;

----

-- name: GetSelectionsByStudent :many
SELECT
	ch.course_id,
	ch.period_id,
	ch.selection_type
FROM choices ch
WHERE ch.student_id = $1
ORDER BY ch.course_id;


-- name: DeleteChoiceByStudentAndCourse :exec
SELECT delete_choice($1, $2);

-- name: DeleteStudentSelection :one
WITH mutation AS MATERIALIZED (
	SELECT delete_choice(
		sqlc.arg(student_id),
		sqlc.arg(course_id)
	)
)
SELECT
	state.course_id,
	state.current_students,
	state.revision AS state_revision
FROM mutation
JOIN course_selection_state state
	ON state.course_id = sqlc.arg(course_id);
