-- The application refuses to start against a schema it does not
-- expect; this is the read behind that check and behind /healthz.
-- name: GetSchemaVersion :one
SELECT version FROM schema_version WHERE singleton;

-- The same version, plus proof that the objects requests actually go
-- through are there.
--
-- The version on its own is a claim the schema makes about itself, not
-- an observation of it, and the two came apart in exactly the way that
-- matters: dropping v_courses and leaving schema_version alone gave a
-- process that started, stayed up, answered /readyz with "ready", and
-- served 500s to every student — so a load balancer held it in
-- rotation indefinitely and nothing escalated. A half-applied
-- migration is that state, and the morning of selection is when
-- migrations are applied.
--
-- Each EXISTS stops at the first row, so this costs a row from each
-- view rather than a scan of it. The rows are not the point: the point
-- is that the statement cannot plan unless every view, and everything
-- each view is built on, is present.
-- name: ReadyProbe :one
SELECT
	(SELECT version FROM schema_version WHERE singleton) AS version,
	EXISTS (SELECT 1 FROM v_grades) AS has_grades,
	EXISTS (SELECT 1 FROM v_grade_requirements) AS has_grade_requirements,
	EXISTS (SELECT 1 FROM v_courses) AS has_courses,
	EXISTS (SELECT 1 FROM v_enrollments) AS has_enrollments,
	EXISTS (SELECT 1 FROM v_student_status) AS has_student_status,
	EXISTS (SELECT 1 FROM v_student_requirements) AS has_student_requirements,
	EXISTS (SELECT 1 FROM v_export_enrollments) AS has_export_enrollments;
