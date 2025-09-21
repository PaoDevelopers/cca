-- name: GetSchemaVersion :one
SELECT version FROM schema_version;

-- name: SetAdminSession :exec
INSERT INTO admins (username, session_token)
VALUES ($2, $1)
ON CONFLICT (username)
DO UPDATE SET session_token = EXCLUDED.session_token;

-- name: SetStudentSession :exec
UPDATE students SET session_token = $1 WHERE id = $2;

-- name: GetStudentBySession :one
SELECT id, name, grade, legal_sex FROM students WHERE session_token = $1;

-- name: GetAdminBySession :one
SELECT id, username FROM admins WHERE session_token = $1;
