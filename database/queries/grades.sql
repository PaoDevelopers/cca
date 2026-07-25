---- Grades

-- name: GetGrades :many
SELECT grade, enabled, max_own_choices
FROM grades
ORDER BY grade;

-- name: NewGrade :exec
INSERT INTO grades (grade, enabled, max_own_choices)
VALUES ($1, false, $2);

-- name: DeleteGrade :exec
DELETE FROM grades
WHERE grade = $1;

-- name: UpdateGradeSettings :execrows
UPDATE grades
SET enabled = $1,
	max_own_choices = $2
WHERE grade = $3;

-- name: SetGradeEnabled :exec
UPDATE grades
SET enabled = $1
WHERE grade = $2;

-- name: GetRequirementGroupsByGrade :many
SELECT
	gr.id,
	gr.min_count,
	COALESCE(ARRAY_AGG(gc.category_id ORDER BY gc.category_id) FILTER (WHERE gc.category_id IS NOT NULL), '{}')::text[] AS category_ids
FROM
	grade_requirement_groups gr
LEFT JOIN
	grade_requirement_group_categories gc ON gr.id = gc.req_group_id
WHERE
	gr.grade = $1
GROUP BY
	gr.id
ORDER BY gr.id;

-- name: GetStudentRequirementProgress :many
SELECT
	requirement.id,
	requirement.min_count,
	COALESCE(ARRAY(
		SELECT category.category_id
		FROM grade_requirement_group_categories category
		WHERE category.req_group_id = requirement.id
		ORDER BY category.category_id
	), '{}')::text[] AS category_ids,
	(
		SELECT COUNT(*)::bigint
		FROM choices student_choice
		JOIN courses selected_course ON selected_course.id = student_choice.course_id
		WHERE student_choice.student_id = sqlc.arg(student_id)
			AND EXISTS (
				SELECT 1
				FROM grade_requirement_group_categories accepted_category
				WHERE accepted_category.req_group_id = requirement.id
					AND accepted_category.category_id = selected_course.category_id
			)
	) AS current_count
FROM grade_requirement_groups requirement
JOIN students student
	ON student.id = sqlc.arg(student_id)
	AND student.grade = requirement.grade
ORDER BY requirement.id;

-- name: GetStudentSelectionStatus :one
SELECT
	grade.enabled,
	grade.max_own_choices,
	(
		SELECT COUNT(*)::bigint
		FROM choices student_choice
		WHERE student_choice.student_id = student.id
			AND student_choice.selection_type = 'normal'
	) AS normal_selection_count,
	schedule.opens_at,
	schedule.closes_at,
	COALESCE(schedule.opened, FALSE)::boolean AS schedule_opened
FROM students student
JOIN grades grade ON grade.grade = student.grade
LEFT JOIN grade_selection_schedules schedule ON schedule.grade = grade.grade
WHERE student.id = sqlc.arg(student_id);

-- name: NewRequirementGroup :exec
WITH new_group AS (
	INSERT INTO grade_requirement_groups (grade, min_count)
	VALUES (sqlc.arg(grade), sqlc.arg(min_count))
	RETURNING id
)
INSERT INTO grade_requirement_group_categories (req_group_id, category_id)
SELECT new_group.id, unnest(sqlc.arg(category_ids)::text[])
FROM new_group;

-- name: DeleteRequirementGroup :exec
DELETE FROM grade_requirement_groups
WHERE id = $1;
