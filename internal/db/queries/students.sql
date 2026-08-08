-- name: GetStudents :many
SELECT id, name, grade_id, legal_sex FROM students ORDER BY id;

-- name: GetStudentStatus :many
SELECT * FROM v_student_status ORDER BY student_id;

-- name: GetStudentStatusByID :one
SELECT * FROM v_student_status WHERE student_id = $1;

-- name: GetStudentRequirements :many
SELECT * FROM v_student_requirements ORDER BY student_id, requirement_category_id;

-- name: GetStudentRequirementsByID :many
SELECT * FROM v_student_requirements
WHERE student_id = $1
ORDER BY requirement_category_id;

-- name: UpsertStudents :exec
SELECT upsert_students($1, $2, $3, $4, $5);

-- name: DeleteStudent :execrows
DELETE FROM students WHERE id = $1;
