-- Global TODO:
-- a few more non-empty string checks


-- All fixed constraints declared in the tables directly are the ones that
-- must always be invariant. The constraints that could be over-ridden
-- depending on the role should be done in the triggers.

-- We use a singleton schema version line to track the schema version.
-- Currently, we have no automatic migrations, and all administrators
-- are expected to manually diff the schema and run necessary migrations.
-- This schema version exists just to make sure that we're not running
-- the application on an incompatible schema.
CREATE TABLE schema_version (
	singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
	version BIGINT NOT NULL CHECK (version > 0)
);
INSERT INTO schema_version (version) VALUES (2);

-- This is the 'gender' field from PowerSchool, but it is more accurately
-- described as legal sex.
CREATE TYPE legal_sex AS ENUM ('F', 'M', 'X');

-- The only difference 'invite' has on top of 'normal', is that the schema will
-- allow inserting 'invite's that don't satisfy typical constraints such as
-- legal sex restrictions, maximum member counts, or year group restrictions. A
-- selection type of 'force' means that the student would not be allowed to
-- remove the selection. Students are only able to add selections of type 'normal';
-- the others may only be added by administrators.
CREATE TYPE selection_type AS ENUM ('normal', 'invite', 'force');

-- Courses may either have 'free' or 'invite_only' membership. Courses with
-- free membership may be chosen by students (as long as the restrictions
-- match), but courses with invite_only would have to be done through
-- the administrator by adding a selection of types 'invite' or 'force'.
CREATE TYPE membership_type AS ENUM ('free', 'invite_only');

-- Grades / year groups.
CREATE TABLE grades (
	grade TEXT PRIMARY KEY,
	-- TODO: Switch 'enabled' to a 'new selections cap', which would not
	-- allow new selections to be made if the cap is reached.
	enabled BOOLEAN NOT NULL DEFAULT FALSE,

	-- A student should not be allowed to make more choices if the number
	-- of choices with selection_type="normal" that they have exceeds the
	-- max_own_choices for their grade.
	-- max_own_choices for each grade should be settable by the admin, next
	-- to where they could set grade enabled status.
	max_own_choices BIGINT NOT NULL DEFAULT 65535 CHECK (max_own_choices >= 0)
);

-- Course categories such as 'Sport', 'Enrichment', 'Art', and 'Culture'
-- at the SJ campus.
CREATE TABLE categories (
	id TEXT PRIMARY KEY CHECK (btrim(id) <> '')
);

-- Grades may have minimum course number requirements. The admin could create
-- an arbitrary number of course categories to add to each 'requirement group',
-- and for each student, each selection from a course that belongs to a
-- category that belongs to a requirement group counts towards satisfying the
-- minimum course count requirement of that requirement group. To add a
-- requirement that 'each student in this grade must have at least n
-- selections', just create a group that includes all categories.
CREATE TABLE grade_requirement_groups (
	id BIGSERIAL PRIMARY KEY,
	grade TEXT NOT NULL REFERENCES grades(grade) ON UPDATE RESTRICT ON DELETE CASCADE,
	min_count BIGINT NOT NULL CHECK (min_count >= 0)
);
CREATE TABLE grade_requirement_group_categories (
	req_group_id BIGINT NOT NULL REFERENCES grade_requirement_groups(id) ON UPDATE CASCADE ON DELETE CASCADE,
	category_id TEXT NOT NULL REFERENCES categories(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
	PRIMARY KEY (req_group_id, category_id)
);

-- The timetable is a fixed 4 x 4 grid. These are reference data, not
-- administrator-managed records: courses may use one or more slots, but the
-- slot catalogue itself never changes at runtime.
CREATE TABLE periods (
	id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
	ordinal SMALLINT NOT NULL UNIQUE CHECK (ordinal BETWEEN 1 AND 16)
);

INSERT INTO periods (id, ordinal) VALUES
	('Monday CCA 1', 1),
	('Monday CCA 2', 2),
	('Monday CCA 3', 3),
	('Monday CCA 4', 4),
	('Tuesday CCA 1', 5),
	('Tuesday CCA 2', 6),
	('Tuesday CCA 3', 7),
	('Tuesday CCA 4', 8),
	('Wednesday CCA 1', 9),
	('Wednesday CCA 2', 10),
	('Wednesday CCA 3', 11),
	('Wednesday CCA 4', 12),
	('Thursday CCA 1', 13),
	('Thursday CCA 2', 14),
	('Thursday CCA 3', 15),
	('Thursday CCA 4', 16);

CREATE FUNCTION reject_period_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	RAISE EXCEPTION 'Timetable periods are fixed reference data'
		USING ERRCODE = 'check_violation', CONSTRAINT = 'periods_fixed';
END;
$$;

-- A statement-level trigger also rejects DELETE/UPDATE statements that happen
-- to match zero rows, and protects against TRUNCATE as well as row-level DML.
CREATE TRIGGER trg_periods_immutable
BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON periods
FOR EACH STATEMENT
EXECUTE FUNCTION reject_period_mutation();

-- Administrators are managed separately from students.
CREATE TABLE admins (
	id BIGSERIAL PRIMARY KEY,
	-- Note: admin usernames are case-sensitive!
	username TEXT NOT NULL UNIQUE CHECK (btrim(username) <> ''),
	-- Each administrator may have a maximum of one session at a time.
	session_token TEXT UNIQUE
);

-- Although students use OpenID Connect for authentication, we still use our
-- own session tokens.
CREATE TABLE students (
	id BIGINT PRIMARY KEY,
	-- If there's a blank student name, let's just let it be.
	-- We only display this name anyway and we don't really process/handle
	-- it in a way that requires it to be unique or usable or anything.
	name TEXT NOT NULL,
	grade TEXT NOT NULL REFERENCES grades(grade) ON UPDATE RESTRICT ON DELETE RESTRICT,
	legal_sex legal_sex NOT NULL,
	session_token TEXT UNIQUE
);

-- TODO: Expiry!

-- Courses
CREATE TABLE courses (
	id TEXT PRIMARY KEY CHECK (btrim(id) <> ''),
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	max_students BIGINT NOT NULL CHECK (max_students >= 0),
	membership membership_type NOT NULL DEFAULT 'free',
	teacher TEXT NOT NULL,
	location TEXT NOT NULL,
	category_id TEXT NOT NULL REFERENCES categories(id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- A course may occupy any number of fixed timetable periods. Periods are
-- atomic slots from the fixed Monday-through-Thursday grid above; they are not
-- free-form time ranges. Every committed course must have at least one
-- assignment; the deferred trigger below lets course creation insert the
-- course and its assignments in one transaction.
CREATE TABLE course_periods (
	course_id TEXT NOT NULL REFERENCES courses(id) ON UPDATE CASCADE ON DELETE CASCADE,
	period_id TEXT NOT NULL REFERENCES periods(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
	PRIMARY KEY (course_id, period_id)
);

CREATE FUNCTION enforce_course_has_period()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	v_course_id TEXT;
BEGIN
	IF TG_TABLE_NAME = 'courses' THEN
		v_course_id := NEW.id;
	ELSE
		v_course_id := OLD.course_id;
	END IF;

	-- A cascading course deletion is valid: by deferred-trigger time the course
	-- row is gone, so there is no remaining invariant to check.
	IF EXISTS (SELECT 1 FROM courses c WHERE c.id = v_course_id)
		AND NOT EXISTS (
			SELECT 1
			FROM course_periods cp
			WHERE cp.course_id = v_course_id
		) THEN
		RAISE EXCEPTION 'Course % has no timetable periods', v_course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'course_requires_period';
	END IF;

	RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER trg_courses_require_period
AFTER INSERT OR UPDATE ON courses
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_course_has_period();

CREATE CONSTRAINT TRIGGER trg_course_periods_require_period
AFTER DELETE OR UPDATE ON course_periods
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_course_has_period();

-- Allowed legal sexes. If none are present then we assume that all legal sexes
-- are allowed for this course.
CREATE TABLE course_allowed_legal_sexes (
	course_id TEXT NOT NULL REFERENCES courses(id) ON UPDATE CASCADE ON DELETE CASCADE,
	legal_sex legal_sex NOT NULL,
	PRIMARY KEY (course_id, legal_sex)
);

-- Allowed grades. If none are present then we assume that all grades are
-- allowed for this course.
CREATE TABLE course_allowed_grades (
	course_id TEXT NOT NULL REFERENCES courses(id) ON UPDATE CASCADE ON DELETE CASCADE,
	grade TEXT NOT NULL REFERENCES grades(grade) ON UPDATE RESTRICT ON DELETE RESTRICT,
	PRIMARY KEY (course_id, grade)
);

-- Choices (student selections and/or invitations)
CREATE TABLE choices (
	student_id BIGINT NOT NULL REFERENCES students(id) ON UPDATE CASCADE ON DELETE RESTRICT,
	course_id TEXT NOT NULL,
	period_id TEXT NOT NULL,
	selection_type selection_type NOT NULL DEFAULT 'normal',
	PRIMARY KEY (student_id, course_id),
	CONSTRAINT choices_period_conflict UNIQUE (student_id, period_id),
	CONSTRAINT choices_course_period_fkey
		FOREIGN KEY (course_id, period_id)
		REFERENCES course_periods(course_id, period_id)
		ON UPDATE CASCADE ON DELETE RESTRICT
);

-- Enforce legal_sex/grade/membership/capacity/selection_window only when
-- selection_type = 'normal'. Invites/forces bypass these checks by design.
CREATE FUNCTION enforce_choice_constraints()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	v_student_grade TEXT;
	v_student_legal_sex legal_sex;
	v_has_grade_list boolean;
	v_grade_allowed boolean;
	v_max bigint;
	v_count bigint;
	v_membership membership_type;
	v_has_legal_sex_list boolean;
	v_legal_sex_allowed boolean;
	v_grade_enabled boolean;
	v_max_own_choices bigint;
	v_student_no_count bigint;
	v_conflicting_course_id TEXT;
	v_conflicting_period_id TEXT;
	v_old_student_id BIGINT;
	v_old_course_id TEXT;
	v_old_period_id TEXT;
BEGIN
	IF TG_OP = 'UPDATE' THEN
		v_old_student_id := OLD.student_id;
		v_old_course_id := OLD.course_id;
		v_old_period_id := OLD.period_id;
	END IF;

	-- Lock one stable row per student before checking their timetable. Without
	-- this lock, two concurrent requests for the same student could both pass
	-- the overlap query and create a double booking.
	SELECT s.grade, s.legal_sex
	INTO v_student_grade, v_student_legal_sex
	FROM students s
	WHERE s.id = NEW.student_id
	FOR UPDATE;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Student % not found', NEW.student_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	-- The lock order for choice mutations is always student, then course. The
	-- course-period foreign key and its row locks serialize choices with slot
	-- removals, without introducing a reverse course-to-student lock order.
	SELECT c.max_students, c.membership
	INTO v_max, v_membership
	FROM courses c
	WHERE c.id = NEW.course_id
	FOR UPDATE;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Course % not found', NEW.course_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	-- A course may offer more than one fixed slot, but a choice selects exactly
	-- one of them. Timetable conflicts remain hard constraints for every
	-- selection type, including invite and force.
	IF TG_OP = 'INSERT'
		OR OLD.student_id IS DISTINCT FROM NEW.student_id
		OR OLD.course_id IS DISTINCT FROM NEW.course_id
		OR OLD.period_id IS DISTINCT FROM NEW.period_id THEN
		IF NOT EXISTS (
			SELECT 1
			FROM course_periods cp
			WHERE cp.course_id = NEW.course_id
				AND cp.period_id = NEW.period_id
		) THEN
			RAISE EXCEPTION 'Period % is not offered by course %', NEW.period_id, NEW.course_id
				USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_course_period';
		END IF;

		SELECT ch.course_id, ch.period_id
		INTO v_conflicting_course_id, v_conflicting_period_id
		FROM choices ch
		WHERE ch.student_id = NEW.student_id
			AND ch.period_id = NEW.period_id
			AND NOT (
				ch.student_id IS NOT DISTINCT FROM v_old_student_id
				AND ch.course_id IS NOT DISTINCT FROM v_old_course_id
				AND ch.period_id IS NOT DISTINCT FROM v_old_period_id
			)
		ORDER BY ch.course_id
		LIMIT 1;

		IF FOUND THEN
			RAISE EXCEPTION 'Course % conflicts with selected course % in period %',
				NEW.course_id, v_conflicting_course_id, v_conflicting_period_id
				USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_period_conflict';
		END IF;
	END IF;

	-- The remaining eligibility/capacity constraints only apply when the
	-- resulting row is a normal selection.
	IF NOT (
		NEW.selection_type = 'normal' AND
		(
			TG_OP = 'INSERT'
			OR OLD.selection_type IS DISTINCT FROM 'normal'
			OR OLD.student_id IS DISTINCT FROM NEW.student_id
			OR OLD.course_id IS DISTINCT FROM NEW.course_id
		)
	) THEN
		RETURN NEW;
	END IF;

	-- Membership (invite-only needs an invitation)
	IF v_membership = 'invite_only' THEN
		RAISE EXCEPTION 'Course % is invite-only; invitation required', NEW.course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_membership';
	END IF;

	-- Legal sex restriction
	SELECT
		EXISTS (SELECT 1 FROM course_allowed_legal_sexes s WHERE s.course_id = NEW.course_id),
		EXISTS (SELECT 1 FROM course_allowed_legal_sexes s WHERE s.course_id = NEW.course_id AND s.legal_sex = v_student_legal_sex)
	INTO v_has_legal_sex_list, v_legal_sex_allowed;

	IF v_has_legal_sex_list AND NOT v_legal_sex_allowed THEN
		RAISE EXCEPTION 'Student % legal sex % not allowed for course %',
			NEW.student_id, v_student_legal_sex, NEW.course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_legal_sex';
	END IF;

	-- Grade restriction
	SELECT
		EXISTS (SELECT 1 FROM course_allowed_grades g WHERE g.course_id = NEW.course_id),
		EXISTS (SELECT 1 FROM course_allowed_grades g WHERE g.course_id = NEW.course_id AND g.grade = v_student_grade)
	INTO v_has_grade_list, v_grade_allowed;
	-- TODO: Consider if we really should allow passing when there are no grades set.
	-- It's actually a bit ugly/inconsistent, in my opinion.

	IF v_has_grade_list AND NOT v_grade_allowed THEN
		RAISE EXCEPTION 'Student % grade % not allowed for course %',
			NEW.student_id, v_student_grade, NEW.course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_grade';
	END IF;

	-- Selection window
	SELECT enabled, max_own_choices
	INTO v_grade_enabled, v_max_own_choices
	FROM grades
	WHERE grade = v_student_grade;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Grade % not found', v_student_grade
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	IF NOT v_grade_enabled THEN
		RAISE EXCEPTION 'Selections are closed for grade %', v_student_grade
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_window';
	END IF;

	-- Own selections cap. Exclude the row being updated, then count NEW once.
	SELECT COUNT(*)::bigint
	INTO v_student_no_count
	FROM choices ch
	WHERE ch.student_id = NEW.student_id
		AND ch.selection_type = 'normal'
		AND NOT (
			ch.student_id IS NOT DISTINCT FROM v_old_student_id
			AND ch.course_id IS NOT DISTINCT FROM v_old_course_id
		);

	IF v_student_no_count + 1 > v_max_own_choices THEN
		RAISE EXCEPTION 'Student % cannot exceed % own selections for grade %',
			NEW.student_id, v_max_own_choices, v_student_grade
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_max_own';
	END IF;

	-- Capacity (after locking the course row)
	SELECT COUNT(*)::bigint
	INTO v_count
	FROM choices ch
	WHERE ch.course_id = NEW.course_id
		AND NOT (
			ch.student_id IS NOT DISTINCT FROM v_old_student_id
			AND ch.course_id IS NOT DISTINCT FROM v_old_course_id
		);

	IF v_count >= v_max THEN
		RAISE EXCEPTION 'Course % is at capacity (% >= %)', NEW.course_id, v_count, v_max
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_capacity';
	END IF;

	RETURN NEW;
END;
$$;
CREATE TRIGGER trg_choices_constraints
BEFORE INSERT OR UPDATE OF student_id, course_id, period_id, selection_type ON choices
FOR EACH ROW
EXECUTE FUNCTION enforce_choice_constraints();

-- A course may gain slots at any time. Removing or moving a slot is allowed
-- only while no choice references that exact course-slot pair. The row lock on
-- course_periods and the choices foreign key serialize concurrent selections.
CREATE FUNCTION enforce_course_period_usage()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	v_course_id TEXT;
	v_period_id TEXT;
BEGIN
	IF TG_OP = 'INSERT' THEN
		v_course_id := NEW.course_id;
		v_period_id := NEW.period_id;
	ELSIF TG_OP = 'DELETE' THEN
		v_course_id := OLD.course_id;
		v_period_id := OLD.period_id;
	ELSIF TG_OP = 'UPDATE' AND OLD.course_id IS DISTINCT FROM NEW.course_id THEN
		RAISE EXCEPTION 'Move a timetable period by deleting and inserting it in one transaction'
			USING ERRCODE = 'check_violation';
	ELSE
		v_course_id := OLD.course_id;
		v_period_id := OLD.period_id;
	END IF;

	IF TG_OP <> 'INSERT' AND EXISTS (
		SELECT 1
		FROM choices ch
		WHERE ch.course_id = v_course_id
			AND ch.period_id = v_period_id
	) THEN
		RAISE EXCEPTION 'Cannot remove timetable period % from course % while it has selections',
			v_period_id, v_course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'course_period_in_use';
	END IF;

	IF TG_OP = 'DELETE' THEN
		RETURN OLD;
	END IF;
	RETURN NEW;
END;
$$;
CREATE TRIGGER trg_course_period_usage
BEFORE INSERT OR UPDATE OR DELETE ON course_periods
FOR EACH ROW
EXECUTE FUNCTION enforce_course_period_usage();




CREATE FUNCTION delete_choice(p_student_id BIGINT, p_course_id TEXT)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_selection_type selection_type;
	v_grade TEXT;
	v_grade_enabled BOOLEAN;
BEGIN
	SELECT selection_type
	INTO v_selection_type
	FROM choices
	WHERE student_id = p_student_id AND course_id = p_course_id
	FOR UPDATE;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'No selection found for student % and course %',
			p_student_id, p_course_id
			USING ERRCODE = 'no_data_found';
	END IF;

	SELECT s.grade, g.enabled
	INTO v_grade, v_grade_enabled
	FROM students s
	JOIN grades g ON g.grade = s.grade
	WHERE s.id = p_student_id;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Student % not found', p_student_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	IF v_grade_enabled IS DISTINCT FROM TRUE THEN
		RAISE EXCEPTION 'Cannot delete selection for student % from disabled grade %',
			p_student_id, v_grade
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_window';
	END IF;

	IF v_selection_type = 'force' THEN
		RAISE EXCEPTION 'Cannot delete forced selection for student % and course %',
			p_student_id, p_course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_force_locked';
	END IF;

	DELETE FROM choices
	WHERE student_id = p_student_id AND course_id = p_course_id;
END;
$$;

CREATE FUNCTION new_selection(
	p_student_id BIGINT,
	p_course_id TEXT,
	p_period_id TEXT,
	p_selection_type selection_type
)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
	PERFORM 1
	FROM students s
	WHERE s.id = p_student_id;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Student % not found', p_student_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	INSERT INTO choices (
		student_id,
		course_id,
		period_id,
		selection_type
	)
	VALUES (
		p_student_id,
		p_course_id,
		p_period_id,
		p_selection_type
	);
END;
$$;

-- Admin batches pre-lock their complete working set before inserting anything.
-- Every batch uses the same global order (all students ascending, then all
-- courses ascending), avoiding cycles with concurrent batches while the row
-- trigger remains the authoritative eligibility/capacity/conflict check.
CREATE FUNCTION new_selections_batch(
	p_student_ids BIGINT[],
	p_course_ids TEXT[],
	p_period_ids TEXT[],
	p_selection_types selection_type[]
)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
	v_count BIGINT;
	v_student_id BIGINT;
	v_course_id TEXT;
	v_requested RECORD;
	v_inserted BIGINT := 0;
BEGIN
	IF p_student_ids IS NULL OR p_course_ids IS NULL OR p_period_ids IS NULL OR p_selection_types IS NULL THEN
		RAISE EXCEPTION 'Selection batch arrays cannot be null'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'selection_batch_shape';
	END IF;

	v_count := cardinality(p_student_ids);
	IF cardinality(p_course_ids) <> v_count
		OR cardinality(p_period_ids) <> v_count
		OR cardinality(p_selection_types) <> v_count THEN
		RAISE EXCEPTION 'Selection batch arrays must have equal lengths'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'selection_batch_shape';
	END IF;

	IF array_position(p_student_ids, NULL) IS NOT NULL
		OR array_position(p_course_ids, NULL) IS NOT NULL
		OR array_position(p_period_ids, NULL) IS NOT NULL
		OR array_position(p_selection_types, NULL) IS NOT NULL
		OR EXISTS (
			SELECT 1 FROM unnest(p_course_ids) requested(value) WHERE btrim(requested.value) = ''
		)
		OR EXISTS (
			SELECT 1 FROM unnest(p_period_ids) requested(value) WHERE btrim(requested.value) = ''
		) THEN
		RAISE EXCEPTION 'Selection batch values cannot be null or blank'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'selection_batch_shape';
	END IF;

	IF EXISTS (
		SELECT 1
		FROM unnest(p_student_ids, p_course_ids) requested(student_id, course_id)
		GROUP BY requested.student_id, requested.course_id
		HAVING count(*) > 1
	) THEN
		RAISE EXCEPTION 'Selection batch contains duplicate student/course pairs'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'selection_batch_duplicate';
	END IF;

	FOR v_student_id IN
		SELECT DISTINCT requested.student_id
		FROM unnest(p_student_ids) requested(student_id)
		ORDER BY requested.student_id
	LOOP
		PERFORM 1
		FROM students s
		WHERE s.id = v_student_id
		FOR UPDATE;

		IF NOT FOUND THEN
			RAISE EXCEPTION 'Student % not found', v_student_id
				USING ERRCODE = 'foreign_key_violation', CONSTRAINT = 'choices_student_id_fkey';
		END IF;
	END LOOP;

	FOR v_course_id IN
		SELECT DISTINCT requested.course_id
		FROM unnest(p_course_ids) requested(course_id)
		ORDER BY requested.course_id
	LOOP
		PERFORM 1
		FROM courses c
		WHERE c.id = v_course_id
		FOR UPDATE;

		IF NOT FOUND THEN
			RAISE EXCEPTION 'Course % not found', v_course_id
				USING ERRCODE = 'foreign_key_violation', CONSTRAINT = 'choices_course_id_fkey';
		END IF;
	END LOOP;

	FOR v_requested IN
		SELECT requested.student_id, requested.course_id, requested.period_id, requested.selection_type
		FROM unnest(p_student_ids, p_course_ids, p_period_ids, p_selection_types)
			requested(student_id, course_id, period_id, selection_type)
		ORDER BY requested.student_id, requested.course_id, requested.period_id
	LOOP
		INSERT INTO choices (student_id, course_id, period_id, selection_type)
		VALUES (v_requested.student_id, v_requested.course_id, v_requested.period_id, v_requested.selection_type);
		v_inserted := v_inserted + 1;
	END LOOP;

	RETURN v_inserted;
END;
$$;

-- TODO: trigger for deletion of choices when forced?


-- Views

CREATE VIEW v_export_selections AS
SELECT
	s.id AS student_id,
	s.name AS student_name,
	s.grade AS grade,
	s.legal_sex AS legal_sex,
	c.id AS course_id,
	c.name AS course_name,
	ch.period_id::TEXT AS periods,
	ch.selection_type AS selection_type
FROM choices ch
JOIN students s ON s.id = ch.student_id
JOIN courses c ON c.id = ch.course_id
ORDER BY s.id, c.id;

-- Indxes

CREATE INDEX IF NOT EXISTS idx_choices_course
	ON choices (course_id);
CREATE INDEX IF NOT EXISTS idx_choices_student_no_only
	ON choices (student_id)
	WHERE selection_type = 'normal';
CREATE INDEX IF NOT EXISTS idx_students_grade
	ON students (grade);
CREATE INDEX IF NOT EXISTS idx_courses_category_id
	ON courses (category_id);
CREATE INDEX IF NOT EXISTS idx_course_periods_period
	ON course_periods (period_id, course_id);
CREATE INDEX IF NOT EXISTS idx_course_allowed_grades_grade
	ON course_allowed_grades (grade);
CREATE INDEX IF NOT EXISTS idx_grade_requirement_groups_grade
	ON grade_requirement_groups (grade);
CREATE INDEX IF NOT EXISTS idx_gr_req_group_categories_category
	ON grade_requirement_group_categories (category_id);
CREATE INDEX IF NOT EXISTS idx_students_session_token
	ON students (session_token);
CREATE INDEX IF NOT EXISTS idx_admins_session_token
	ON admins (session_token);
