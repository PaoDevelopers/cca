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

---- Admin dashboard

-- name: GetAdminDashboardCounts :one
SELECT
	(SELECT COUNT(*) FROM courses)::bigint AS course_count,
	(
		SELECT COUNT(*)
		FROM courses course
		WHERE NOT EXISTS (
			SELECT 1
			FROM course_periods assignment
			WHERE assignment.course_id = course.id
		)
	)::bigint AS courses_without_timetable,
	(SELECT COUNT(*) FROM students)::bigint AS student_count;

---- Categories

-- name: GetCategories :many
SELECT id
FROM categories
ORDER BY id;

-- name: NewCategory :exec
INSERT INTO categories (id)
VALUES ($1);

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1;

---- Periods

-- name: GetPeriods :many
SELECT id
FROM periods
ORDER BY ordinal;

-- name: GetCoursePeriodAssignments :many
SELECT cp.course_id, cp.period_id
FROM course_periods cp
JOIN periods p ON p.id = cp.period_id
ORDER BY cp.course_id, p.ordinal;

-- name: GetCoursePeriodsByCourse :many
SELECT cp.period_id
FROM course_periods cp
JOIN periods p ON p.id = cp.period_id
WHERE cp.course_id = $1
ORDER BY p.ordinal;

-- name: AddCoursePeriod :exec
INSERT INTO course_periods (course_id, period_id)
VALUES ($1, $2)
ON CONFLICT (course_id, period_id) DO NOTHING;

-- name: DeleteCoursePeriodsExcept :exec
DELETE FROM course_periods
WHERE course_id = sqlc.arg(course_id)
	AND NOT (period_id = ANY(sqlc.arg(period_ids)::text[]));

-- name: DeleteCoursePeriods :exec
DELETE FROM course_periods
WHERE course_id = $1;

---- Courses

-- name: GetCourses :many
SELECT
	id,
	name,
	description,
	ARRAY(
		SELECT cp.period_id
		FROM course_periods cp
		JOIN periods p ON p.id = cp.period_id
		WHERE cp.course_id = courses.id
		ORDER BY p.ordinal
	)::text[] AS period_ids,
		max_students,
		membership,
		teacher,
		location,
		category_id,
		state.current_students,
		state.revision AS state_revision
FROM courses
JOIN course_selection_state state ON state.course_id = courses.id
ORDER BY id;

-- name: NewCourse :exec
INSERT INTO courses (
	id,
	name,
	description,
	max_students,
	membership,
	teacher,
	location,
	category_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateCourse :execrows
UPDATE courses
SET
	name = $2,
	description = $3,
	max_students = $4,
	membership = $5,
	teacher = $6,
	location = $7,
	category_id = $8
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

-- name: GetCourseStatesByIDs :many
WITH requested AS (
	SELECT unnest($1::text[]) AS id
)
SELECT
	req.id::text AS id,
	state.current_students,
	state.revision AS state_revision
FROM requested req
JOIN course_selection_state state ON state.course_id = req.id
ORDER BY req.id;

-- Get the complete student-facing catalogue from one PostgreSQL snapshot.
-- Availability and removal state are deliberately calculated here rather
-- than in Go so this read model cannot drift from the database rules.
-- name: GetStudentCourseCatalog :many
WITH student_context AS MATERIALIZED (
	SELECT
		s.id AS student_id,
		s.grade,
		s.legal_sex,
		g.enabled AS grade_enabled,
		g.max_own_choices,
		COUNT(own_choice.course_id) FILTER (
			WHERE own_choice.selection_type = 'normal'
		)::bigint AS normal_selection_count
	FROM students s
	JOIN grades g ON g.grade = s.grade
	LEFT JOIN choices own_choice ON own_choice.student_id = s.id
	WHERE s.id = sqlc.arg(student_id)
	GROUP BY s.id, s.grade, s.legal_sex, g.enabled, g.max_own_choices
), student_choices AS MATERIALIZED (
	SELECT choice.course_id, choice.period_id, choice.selection_type
	FROM choices choice
	JOIN student_context student ON student.student_id = choice.student_id
), occupied_periods AS MATERIALIZED (
	SELECT COALESCE(array_agg(choice.period_id), '{}')::text[] AS period_ids
	FROM student_choices choice
), course_period_sets AS MATERIALIZED (
	SELECT
		assignment.course_id,
		array_agg(assignment.period_id ORDER BY period.ordinal)::text[] AS period_ids
	FROM course_periods assignment
	JOIN periods period ON period.id = assignment.period_id
	GROUP BY assignment.course_id
), course_legal_sex_sets AS MATERIALIZED (
	SELECT
		allowed.course_id,
		array_agg(allowed.legal_sex::text ORDER BY allowed.legal_sex)::text[] AS legal_sexes
	FROM course_allowed_legal_sexes allowed
	GROUP BY allowed.course_id
), course_grade_sets AS MATERIALIZED (
	SELECT
		allowed.course_id,
		array_agg(allowed.grade ORDER BY allowed.grade)::text[] AS grades
	FROM course_allowed_grades allowed
	GROUP BY allowed.course_id
), course_state AS MATERIALIZED (
	SELECT
		c.id,
		c.name,
		c.description,
		COALESCE(periods.period_ids, '{}')::text[] AS period_ids,
		c.max_students,
		realtime.current_students,
		realtime.revision AS state_revision,
		c.membership,
		c.teacher,
		c.location,
		c.category_id,
		COALESCE(legal_sexes.legal_sexes, '{}')::text[] AS allowed_legal_sexes,
		COALESCE(grades.grades, '{}')::text[] AS allowed_grades,
		(student_choice.course_id IS NOT NULL)::boolean AS selected,
		student_choice.period_id AS selected_period_id,
		student_choice.selection_type,
		student.grade_enabled,
		student.max_own_choices,
		student.normal_selection_count,
		student.student_id,
		student.grade AS student_grade,
		student.legal_sex AS student_legal_sex
	FROM courses c
	CROSS JOIN student_context student
	JOIN course_selection_state realtime ON realtime.course_id = c.id
	LEFT JOIN course_period_sets periods ON periods.course_id = c.id
	LEFT JOIN course_legal_sex_sets legal_sexes ON legal_sexes.course_id = c.id
	LEFT JOIN course_grade_sets grades ON grades.course_id = c.id
	LEFT JOIN student_choices student_choice ON student_choice.course_id = c.id
), course_with_base_reasons AS (
	SELECT
		course.*,
		COALESCE(
			(
				SELECT jsonb_agg(reason.reason ORDER BY reason.priority)
				FROM (
					VALUES
						(
							10,
							CASE
								WHEN NOT course.selected AND cardinality(course.period_ids) = 0
									THEN jsonb_build_object(
										'code', 'no_periods',
										'message', 'This CCA does not have a timetable yet.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							20,
							CASE
								WHEN NOT course.selected
									AND course.current_students >= course.max_students
									THEN jsonb_build_object(
										'code', 'course_full',
										'message', 'This CCA is full.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							30,
							CASE
								WHEN NOT course.selected AND course.membership = 'invite_only'
									THEN jsonb_build_object(
										'code', 'invite_only',
										'message', 'This CCA is invitation only.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							40,
							CASE
								WHEN NOT course.selected
									AND cardinality(course.allowed_legal_sexes) > 0
									AND NOT (
										course.student_legal_sex::text = ANY(course.allowed_legal_sexes)
									)
									THEN jsonb_build_object(
										'code', 'legal_sex_restricted',
										'message', 'This CCA is not available for you.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							50,
							CASE
								WHEN NOT course.selected
									AND cardinality(course.allowed_grades) > 0
									AND NOT (course.student_grade = ANY(course.allowed_grades))
									THEN jsonb_build_object(
										'code', 'grade_restricted',
										'message', 'This CCA is not available for your grade.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							60,
							CASE
								WHEN NOT course.selected AND NOT course.grade_enabled
									THEN jsonb_build_object(
										'code', 'selections_closed',
										'message', 'Selections are currently closed for your grade.'
									)
								ELSE NULL::jsonb
							END
						),
						(
							70,
							CASE
								WHEN NOT course.selected
									AND course.grade_enabled
									AND course.normal_selection_count >= course.max_own_choices
									THEN jsonb_build_object(
										'code', 'choice_limit',
										'message', format(
											'You have reached your limit of %s own selections.',
											course.max_own_choices
										)
									)
								ELSE NULL::jsonb
							END
						)
				) reason(priority, reason)
				WHERE reason.reason IS NOT NULL
			),
			'[]'::jsonb
		) AS base_block_reasons
	FROM course_state course
), course_with_period_state AS (
	SELECT
		course.*,
		(CASE
				WHEN course.selected THEN ARRAY[course.selected_period_id]::text[]
				WHEN jsonb_array_length(course.base_block_reasons) > 0 THEN '{}'::text[]
				ELSE ARRAY(
					SELECT proposed_period_id
					FROM unnest(course.period_ids) proposed(proposed_period_id)
					WHERE NOT (
						proposed.proposed_period_id = ANY(occupied.period_ids)
					)
				)
			END)::text[] AS available_period_ids,
			COALESCE(conflicts.block_reasons, '[]'::jsonb) AS schedule_block_reasons
	FROM course_with_base_reasons course
	CROSS JOIN occupied_periods occupied
	LEFT JOIN LATERAL (
		SELECT jsonb_agg(
			jsonb_build_object(
				'code', 'schedule_conflict',
				'message', format(
					'Clashes with %s in %s.',
					conflict.conflicting_course_id,
					conflict.period_ids[1]
				),
				'period_ids', to_jsonb(conflict.period_ids),
				'conflicting_course_id', conflict.conflicting_course_id
			)
			ORDER BY conflict.conflicting_course_id
		) AS block_reasons
		FROM (
			SELECT
				selected.course_id AS conflicting_course_id,
				array_agg(selected.period_id ORDER BY period.ordinal)::text[] AS period_ids
			FROM student_choices selected
			JOIN periods period ON period.id = selected.period_id
			WHERE NOT course.selected
				AND selected.period_id = ANY(course.period_ids)
				AND selected.course_id <> course.id
			GROUP BY selected.course_id
		) conflict
	) conflicts ON TRUE
)
SELECT
	id,
	name,
	description,
	period_ids,
	max_students,
	current_students,
	state_revision,
	membership,
	teacher,
	location,
	category_id,
	allowed_legal_sexes,
	allowed_grades,
	selected::boolean AS selected,
	selected_period_id,
	available_period_ids,
	selection_type,
	(selected OR cardinality(available_period_ids) > 0)::boolean AS available,
	(base_block_reasons || schedule_block_reasons)::jsonb AS block_reasons,
	COALESCE(
		selected AND selection_type <> 'force' AND grade_enabled,
		FALSE
	)::boolean AS removable,
	CASE
		WHEN selected AND selection_type = 'force'
			THEN 'A forced selection can only be removed by an administrator.'
		WHEN selected AND NOT grade_enabled
			THEN 'Selections are closed for your grade.'
		ELSE ''
	END::text AS removal_block_reason
FROM course_with_period_state
ORDER BY id;

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
	sqlc.arg(selection_types)::selection_type[]
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
