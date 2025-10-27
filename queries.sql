---- Session Management

-- name: GetSchemaVersion :one
SELECT version
FROM schema_version;

-- name: SetAdminSession :exec
INSERT INTO admins (username, session_token)
VALUES ($2, $1)
ON CONFLICT (username)
DO UPDATE SET session_token = EXCLUDED.session_token;

-- name: SetStudentSession :one
UPDATE students
SET session_token = $1
WHERE id = $2
RETURNING id;

-- name: GetStudentBySession :one
SELECT id, name, grade, legal_sex
FROM students
WHERE session_token = $1;

-- name: GetAdminBySession :one
SELECT id, username
FROM admins
WHERE session_token = $1;

---- Categories

-- name: GetCategories :many
SELECT id
FROM categories;

-- name: NewCategory :exec
INSERT INTO categories (id)
VALUES ($1);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1;

---- Periods

-- name: GetPeriods :many
SELECT id
FROM periods;

-- name: NewPeriod :exec
INSERT INTO periods (id)
VALUES ($1);

-- name: DeletePeriod :exec
DELETE FROM periods
WHERE id = $1;

---- Grades

-- name: GetGrades :many
SELECT grade, enabled
FROM grades;

-- name: NewGrade :exec
INSERT INTO grades (grade, enabled)
VALUES ($1, false);

-- name: DeleteGrade :exec
DELETE FROM grades
WHERE grade = $1;

-- name: SetGradeEnabled :exec
UPDATE grades
SET enabled = $1
WHERE grade = $2;

-- name: SetGradesBulkEnabledUpdate :exec
UPDATE grades
SET enabled = 
	CASE
		WHEN COALESCE(array_length($1::text[], 1), 0) = 0 THEN FALSE
		ELSE grade = ANY($1::text[])
	END;

-- name: GetRequirementGroupsByGrade :many
SELECT
	gr.id,
	gr.min_count,
	COALESCE(ARRAY_AGG(gc.category_id) FILTER (WHERE gc.category_id IS NOT NULL), '{}') AS category_ids
FROM
	grade_requirement_groups gr
LEFT JOIN
	grade_requirement_group_categories gc ON gr.id = gc.req_group_id
WHERE
	gr.grade = $1
GROUP BY
	gr.id;

-- name: NewRequirementGroup :exec
WITH new_group AS (
	INSERT INTO grade_requirement_groups (grade, min_count)
	VALUES ($1, $2)
	RETURNING id
)
INSERT INTO grade_requirement_group_categories (req_group_id, category_id)
SELECT new_group.id, unnest($3::text[])
FROM new_group;

-- name: DeleteRequirementGroup :exec
DELETE FROM grade_requirement_groups
WHERE id = $1;

---- Students

-- name: NewStudent :exec
INSERT INTO students (id, name, grade, legal_sex)
VALUES ($1, $2, $3, $4);

-- name: GetStudentByID :one
SELECT id, name, grade, legal_sex
FROM students
WHERE id = $1;
