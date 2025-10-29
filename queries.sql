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

---- Courses

-- name: GetCourses :many
SELECT
	id,
	name,
	description,
	period,
	max_students,
	membership,
	teacher,
	location,
	category_id,
	(SELECT COUNT(*) FROM choices ch WHERE ch.course_id = courses.id) AS current_students
FROM courses
ORDER BY id;

-- name: NewCourse :exec
INSERT INTO courses (
	id,
	name,
	description,
	period,
	max_students,
	membership,
	teacher,
	location,
	category_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateCourse :exec
UPDATE courses
SET
	name = $2,
	description = $3,
	period = $4,
	max_students = $5,
	membership = $6,
	teacher = $7,
	location = $8,
	category_id = $9
WHERE id = $1;

-- name: DeleteCourse :exec
DELETE FROM courses
WHERE id = $1;

-- name: AddCourseAllowedLegalSex :exec
INSERT INTO course_allowed_legal_sexes (course_id, legal_sex)
VALUES ($1, $2);

-- name: AddCourseAllowedGrade :exec
INSERT INTO course_allowed_grades (course_id, grade)
VALUES ($1, $2);

-- name: GetCourseAllowedLegalSexes :many
SELECT course_id, legal_sex
FROM course_allowed_legal_sexes
ORDER BY course_id, legal_sex;

-- name: GetCourseAllowedGrades :many
SELECT course_id, grade
FROM course_allowed_grades
ORDER BY course_id, grade;

-- name: DeleteCourseAllowedLegalSexes :exec
DELETE FROM course_allowed_legal_sexes
WHERE course_id = $1;

-- name: DeleteCourseAllowedGrades :exec
DELETE FROM course_allowed_grades
WHERE course_id = $1;

-- name: GetCourseCountsByIDs :many
WITH requested AS (
	SELECT unnest($1::text[]) AS id
)
SELECT
	req.id::text AS id,
	COALESCE(COUNT(ch.course_id), 0)::bigint AS current_students
FROM requested req
LEFT JOIN choices ch ON ch.course_id = req.id
GROUP BY req.id;

---- Grades

-- name: GetGrades :many
SELECT grade, enabled, max_own_choices
FROM grades;

-- name: NewGrade :exec
INSERT INTO grades (grade, enabled, max_own_choices)
VALUES ($1, false, $2);

-- name: DeleteGrade :exec
DELETE FROM grades
WHERE grade = $1;

-- name: UpdateGradeSettings :exec
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

-- name: GetStudents :many
SELECT id, name, grade, legal_sex, session_token
FROM students
ORDER BY id;

-- name: NewStudent :exec
INSERT INTO students (id, name, grade, legal_sex)
VALUES ($1, $2, $3, $4);

-- name: GetStudentByID :one
SELECT id, name, grade, legal_sex
FROM students
WHERE id = $1;

-- name: UpdateStudent :exec
UPDATE students
SET name = $2, grade = $3, legal_sex = $4
WHERE id = $1;

-- name: DeleteStudent :exec
DELETE FROM students
WHERE id = $1;

---- Selections

-- name: GetSelections :many
SELECT
	ch.student_id,
	s.name AS student_name,
	s.grade AS student_grade,
	ch.course_id,
	c.name AS course_name,
	ch.period,
	ch.selection_type
FROM choices ch
JOIN students s ON s.id = ch.student_id
JOIN courses c ON c.id = ch.course_id
ORDER BY ch.student_id, ch.period;

-- name: NewSelection :exec
INSERT INTO choices (
	student_id,
	course_id,
	period,
	selection_type
)
SELECT
	$1,
	$2,
	c.period,
	$3
FROM courses c
WHERE c.id = $2;

-- name: NewSelectionsBulk :exec
WITH student_ids AS (
	SELECT DISTINCT unnest($1::bigint[]) AS student_id
),
course_ids AS (
	SELECT DISTINCT c.id, c.period
	FROM courses c
	WHERE c.id = ANY($2::text[])
),
to_insert AS (
	SELECT s.student_id, c.id AS course_id, c.period
	FROM student_ids s
	CROSS JOIN course_ids c
)
INSERT INTO choices (
	student_id,
	course_id,
	period,
	selection_type
)
SELECT
	ti.student_id,
	ti.course_id,
	ti.period,
	$3
FROM to_insert ti;

-- name: UpdateSelection :exec
UPDATE choices AS ch
SET
	course_id = $3,
	period = c.period,
	selection_type = $4
FROM courses c
WHERE
	ch.student_id = $1
	AND ch.period = $2
	AND c.id = $3;

-- name: DeleteSelection :exec
DELETE FROM choices
WHERE student_id = $1 AND period = $2;

----

-- name: GetSelectionsByStudent :many
SELECT course_id, period, selection_type
FROM choices
WHERE student_id = $1;


-- name: GetSelectionCourseByStudentAndPeriod :one
SELECT course_id
FROM choices
WHERE student_id = $1 AND period = $2;


-- name: DeleteChoiceByStudentAndCourse :exec
SELECT delete_choice($1, $2);
