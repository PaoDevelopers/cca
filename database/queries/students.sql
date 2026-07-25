---- Students

-- name: GetStudents :many
SELECT id, name, grade, legal_sex, session_token
FROM students
ORDER BY id;

-- name: GetStudentsForAdmin :many
SELECT id, name, grade, legal_sex
FROM students
ORDER BY id;

-- name: NewStudent :exec
INSERT INTO students (id, name, grade, legal_sex)
VALUES ($1, $2, $3, $4);

-- name: GetStudentByID :one
SELECT id, name, grade, legal_sex
FROM students
WHERE id = $1;

-- name: UpdateStudent :execrows
UPDATE students
SET name = $2, grade = $3, legal_sex = $4
WHERE id = $1;

-- name: DeleteStudent :exec
DELETE FROM students
WHERE id = $1;
