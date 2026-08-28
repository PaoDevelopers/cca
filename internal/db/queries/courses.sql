-- name: GetCourses :many
SELECT * FROM v_courses ORDER BY id;

-- p_max_students is named rather than positional because it is the
-- one nullable argument here, and sqlc reads a function parameter as
-- NOT NULL unless it is told: NULL is no cap, and passing it is the
-- only way to say so.
-- name: CreateCourse :exec
SELECT create_course($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	sqlc.narg(p_max_students), $12, $13, $14);

-- name: UpdateCourse :exec
SELECT update_course($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	sqlc.narg(p_max_students), $12, $13, $14, $15);

-- name: RenameCourseID :execrows
UPDATE courses SET id = $2 WHERE id = $1;

-- name: DeleteCourse :exec
SELECT delete_course($1);

-- The realtime mirror's refresh: the enrollee count of the courses
-- that have just been written, and only those. Reads current_students
-- from v_courses so the browser's count and the capacity rule cannot
-- come from two different definitions.
-- name: GetCourseCountsByIDs :many
SELECT v.id, v.current_students
FROM v_courses v
WHERE v.id = ANY (sqlc.arg(course_ids)::TEXT[]);

-- name: UpsertCourses :exec
SELECT upsert_courses($1, $2);
