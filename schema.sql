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

-- One future selection-access window may be configured per grade. Rows that
-- share a batch_id were created together and are presented as one schedule in
-- the admin UI. Completed and cancelled rows are deleted; this table stores
-- operational state, not an audit history.
CREATE SEQUENCE grade_selection_schedule_batch_id_seq;
CREATE TABLE grade_selection_schedules (
	grade TEXT PRIMARY KEY REFERENCES grades(grade) ON UPDATE CASCADE ON DELETE CASCADE,
	batch_id BIGINT NOT NULL,
	opens_at TIMESTAMPTZ NOT NULL,
	closes_at TIMESTAMPTZ,
	opened BOOLEAN NOT NULL DEFAULT FALSE,
	CONSTRAINT grade_selection_schedule_range
		CHECK (closes_at IS NULL OR closes_at > opens_at),
	CONSTRAINT grade_selection_schedule_open_state
		CHECK (NOT opened OR closes_at IS NOT NULL)
);
CREATE INDEX idx_grade_selection_schedules_batch
	ON grade_selection_schedules (batch_id, grade);
CREATE INDEX idx_grade_selection_schedules_due
	ON grade_selection_schedules (opened, opens_at, closes_at);

-- Each application instance polls this revision so scheduled access changes
-- are propagated to WebSocket clients even when another instance performed
-- the database update.
CREATE TABLE selection_access_state (
	singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
	revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0)
);
INSERT INTO selection_access_state (singleton, revision) VALUES (TRUE, 0);

CREATE FUNCTION bump_selection_access_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF OLD.enabled IS DISTINCT FROM NEW.enabled THEN
		UPDATE selection_access_state
		SET revision = revision + 1
		WHERE singleton = TRUE;
	END IF;
	RETURN NEW;
END;
$$;
CREATE TRIGGER trg_grades_selection_access_revision
AFTER UPDATE OF enabled ON grades
FOR EACH ROW
EXECUTE FUNCTION bump_selection_access_revision();

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

-- Course capacity is read on every selection attempt and displayed to every
-- connected student. Keep the authoritative count and a monotonic revision in
-- PostgreSQL so capacity checks do not repeatedly count choices and WebSocket
-- clients can reject out-of-order updates.
CREATE SEQUENCE course_selection_state_revision_seq AS BIGINT;
CREATE TABLE course_selection_state (
	course_id TEXT PRIMARY KEY REFERENCES courses(id) ON UPDATE CASCADE ON DELETE CASCADE,
	current_students BIGINT NOT NULL DEFAULT 0 CHECK (current_students >= 0),
	revision BIGINT NOT NULL DEFAULT nextval('course_selection_state_revision_seq')
		CHECK (revision > 0)
);
INSERT INTO course_selection_state (course_id, current_students)
SELECT c.id, COUNT(ch.student_id)::bigint
FROM courses c
LEFT JOIN choices ch ON ch.course_id = c.id
GROUP BY c.id;

CREATE FUNCTION create_course_selection_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	INSERT INTO course_selection_state (course_id)
	VALUES (NEW.id);
	RETURN NEW;
END;
$$;
CREATE TRIGGER trg_courses_selection_state
AFTER INSERT ON courses
FOR EACH ROW
EXECUTE FUNCTION create_course_selection_state();

CREATE FUNCTION bump_course_selection_state()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		UPDATE course_selection_state
		SET
			current_students = current_students + 1,
			revision = nextval('course_selection_state_revision_seq')
		WHERE course_id = NEW.course_id;
		IF NOT FOUND THEN
			RAISE EXCEPTION 'Course selection state missing for %', NEW.course_id;
		END IF;
	ELSIF TG_OP = 'DELETE' THEN
		UPDATE course_selection_state
		SET
			current_students = current_students - 1,
			revision = nextval('course_selection_state_revision_seq')
		WHERE course_id = OLD.course_id;
		IF NOT FOUND THEN
			RAISE EXCEPTION 'Course selection state missing for %', OLD.course_id;
		END IF;
	ELSIF OLD.course_id IS DISTINCT FROM NEW.course_id THEN
		-- Lock both state rows in the same order used for course rows by the
		-- choice constraint trigger, preventing cross-course update deadlocks.
		PERFORM 1
		FROM course_selection_state
		WHERE course_id IN (OLD.course_id, NEW.course_id)
		ORDER BY course_id
		FOR UPDATE;

		UPDATE course_selection_state
		SET
			current_students = current_students - 1,
			revision = nextval('course_selection_state_revision_seq')
		WHERE course_id = OLD.course_id;
		UPDATE course_selection_state
		SET
			current_students = current_students + 1,
			revision = nextval('course_selection_state_revision_seq')
		WHERE course_id = NEW.course_id;
	END IF;
	RETURN NULL;
END;
$$;
CREATE TRIGGER trg_choices_selection_state
AFTER INSERT OR UPDATE OF course_id OR DELETE ON choices
FOR EACH ROW
EXECUTE FUNCTION bump_course_selection_state();

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

	-- The lock order for choice mutations is always student, then every affected
	-- course in lexical order. The global order also prevents two administrators
	-- swapping selections between courses from deadlocking.
	IF TG_OP = 'UPDATE' AND OLD.course_id IS DISTINCT FROM NEW.course_id THEN
		PERFORM 1
		FROM courses c
		WHERE c.id IN (OLD.course_id, NEW.course_id)
		ORDER BY c.id
		FOR UPDATE;
	END IF;

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
	WHERE grade = v_student_grade
	FOR SHARE;

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

	-- Capacity (after locking the course row). The state table is maintained by
	-- an AFTER trigger in the same transaction as every choice mutation.
	SELECT state.current_students
	INTO v_count
	FROM course_selection_state state
	WHERE state.course_id = NEW.course_id;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Course selection state missing for %', NEW.course_id;
	END IF;

	IF TG_OP = 'UPDATE' AND OLD.course_id IS NOT DISTINCT FROM NEW.course_id THEN
		v_count := v_count - 1;
	END IF;

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
	-- Use the same student-then-course lock order as inserts and updates so a
	-- deletion cannot race capacity state or a concurrent choice mutation.
	PERFORM 1
	FROM students s
	WHERE s.id = p_student_id
	FOR UPDATE;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Student % not found', p_student_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	PERFORM 1
	FROM courses c
	WHERE c.id = p_course_id
	FOR UPDATE;

	IF NOT FOUND THEN
		RAISE EXCEPTION 'Course % not found', p_course_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

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
	WHERE s.id = p_student_id
	FOR SHARE OF g;

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

-- Immediate access changes and scheduled changes share one advisory lock. This
-- keeps manual overrides, schedule edits, and the background runner atomic
-- without requiring pg_cron or any database extension.
CREATE FUNCTION set_grade_selection_access(
	p_grades TEXT[],
	p_enabled BOOLEAN
)
RETURNS TABLE (
	updated_count BIGINT,
	cancelled_schedule_count BIGINT
)
LANGUAGE plpgsql
AS $$
DECLARE
	v_grades TEXT[];
	v_found BIGINT;
	v_updated BIGINT := 0;
	v_cancelled BIGINT := 0;
BEGIN
	IF p_grades IS NULL
		OR cardinality(p_grades) = 0
		OR EXISTS (
			SELECT 1
			FROM unnest(p_grades) requested(grade)
			WHERE requested.grade IS NULL OR btrim(requested.grade) = ''
		) THEN
		RAISE EXCEPTION 'Choose at least one valid grade'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'grade_access_grades';
	END IF;

	SELECT array_agg(DISTINCT btrim(requested.grade) ORDER BY btrim(requested.grade))
	INTO v_grades
	FROM unnest(p_grades) requested(grade);

	PERFORM pg_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	);

	SELECT count(*)
	INTO v_found
	FROM grades g
	WHERE g.grade = ANY(v_grades);
	IF v_found <> cardinality(v_grades) THEN
		RAISE EXCEPTION 'One or more grades do not exist'
			USING ERRCODE = 'foreign_key_violation', CONSTRAINT = 'grade_access_grade';
	END IF;

	PERFORM 1
	FROM grades g
	WHERE g.grade = ANY(v_grades)
	ORDER BY g.grade
	FOR UPDATE;

	WITH cancelled AS (
		DELETE FROM grade_selection_schedules schedule
		WHERE schedule.grade = ANY(v_grades)
		RETURNING 1
	)
	SELECT count(*) INTO v_cancelled FROM cancelled;

	UPDATE grades g
	SET enabled = p_enabled
	WHERE g.grade = ANY(v_grades);
	GET DIAGNOSTICS v_updated = ROW_COUNT;

	RETURN QUERY SELECT v_updated, v_cancelled;
END;
$$;

-- Updating a single grade's limit remains compatible with the existing API.
-- A real access-state change is a manual override and therefore cancels that
-- grade's pending or active schedule; changing only the limit does not.
CREATE FUNCTION update_grade_settings_with_schedule(
	p_grade TEXT,
	p_enabled BOOLEAN,
	p_max_own_choices BIGINT
)
RETURNS BOOLEAN
LANGUAGE plpgsql
AS $$
DECLARE
	v_enabled BOOLEAN;
BEGIN
	IF p_grade IS NULL OR btrim(p_grade) = '' OR p_max_own_choices < 0 THEN
		RAISE EXCEPTION 'A grade and a non-negative selection limit are required'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'grade_settings';
	END IF;

	PERFORM pg_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	);
	SELECT grade.enabled
	INTO v_enabled
	FROM grades grade
	WHERE grade.grade = btrim(p_grade)
	FOR UPDATE;
	IF NOT FOUND THEN
		RETURN FALSE;
	END IF;

	IF p_enabled IS NOT NULL AND v_enabled IS DISTINCT FROM p_enabled THEN
		DELETE FROM grade_selection_schedules schedule
		WHERE schedule.grade = btrim(p_grade);
	END IF;
	UPDATE grades grade
	SET enabled = COALESCE(p_enabled, v_enabled),
		max_own_choices = p_max_own_choices
	WHERE grade.grade = btrim(p_grade);
	RETURN TRUE;
END;
$$;

CREATE FUNCTION save_grade_selection_schedule(
	p_batch_id BIGINT,
	p_grades TEXT[],
	p_opens_at TIMESTAMPTZ,
	p_closes_at TIMESTAMPTZ,
	p_replace_existing BOOLEAN
)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
	v_grades TEXT[];
	v_existing_grades TEXT[];
	v_conflicting_grades TEXT[];
	v_existing_opens_at TIMESTAMPTZ;
	v_existing_opened BOOLEAN := FALSE;
	v_conflicting_opened BOOLEAN := FALSE;
	v_found BIGINT;
	v_batch_id BIGINT;
BEGIN
	IF p_grades IS NULL
		OR cardinality(p_grades) = 0
		OR EXISTS (
			SELECT 1
			FROM unnest(p_grades) requested(grade)
			WHERE requested.grade IS NULL OR btrim(requested.grade) = ''
		) THEN
		RAISE EXCEPTION 'Choose at least one valid grade'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'grade_schedule_grades';
	END IF;
	IF p_opens_at IS NULL THEN
		RAISE EXCEPTION 'Choose an opening date and time'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'grade_schedule_opens_at';
	END IF;
	IF p_closes_at IS NOT NULL AND p_closes_at <= p_opens_at THEN
		RAISE EXCEPTION 'Closing time must be after opening time'
			USING ERRCODE = 'check_violation', CONSTRAINT = 'grade_selection_schedule_range';
	END IF;

	SELECT array_agg(DISTINCT btrim(requested.grade) ORDER BY btrim(requested.grade))
	INTO v_grades
	FROM unnest(p_grades) requested(grade);

	PERFORM pg_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	);

	SELECT count(*)
	INTO v_found
	FROM grades g
	WHERE g.grade = ANY(v_grades);
	IF v_found <> cardinality(v_grades) THEN
		RAISE EXCEPTION 'One or more grades do not exist'
			USING ERRCODE = 'foreign_key_violation', CONSTRAINT = 'grade_schedule_grade';
	END IF;

	PERFORM 1
	FROM grades g
	WHERE g.grade = ANY(v_grades)
	ORDER BY g.grade
	FOR UPDATE;

	IF p_batch_id IS NOT NULL THEN
		SELECT
			array_agg(schedule.grade ORDER BY schedule.grade),
			bool_and(schedule.opened),
			min(schedule.opens_at)
		INTO v_existing_grades, v_existing_opened, v_existing_opens_at
		FROM grade_selection_schedules schedule
		WHERE schedule.batch_id = p_batch_id;

		IF v_existing_grades IS NULL THEN
			RAISE EXCEPTION 'Selection schedule % not found', p_batch_id
				USING ERRCODE = 'no_data_found', CONSTRAINT = 'grade_schedule_batch';
		END IF;

		-- Once a window has opened, editing may only move its closing time. This
		-- prevents an edit from silently closing or re-opening active grades.
		IF v_existing_opened THEN
			IF v_grades IS DISTINCT FROM v_existing_grades
				OR p_opens_at IS DISTINCT FROM v_existing_opens_at
				OR p_closes_at IS NULL
				OR p_closes_at <= CURRENT_TIMESTAMP THEN
				RAISE EXCEPTION 'An open selection window may only change to a future closing time'
					USING ERRCODE = 'check_violation', CONSTRAINT = 'grade_schedule_opened';
			END IF;

			UPDATE grade_selection_schedules
			SET closes_at = p_closes_at
			WHERE batch_id = p_batch_id;
			RETURN p_batch_id;
		END IF;
	END IF;

	IF p_opens_at <= CURRENT_TIMESTAMP THEN
		RAISE EXCEPTION 'Opening time must be in the future'
			USING ERRCODE = 'check_violation', CONSTRAINT = 'grade_schedule_in_past';
	END IF;

	SELECT array_agg(schedule.grade ORDER BY schedule.grade)
	INTO v_conflicting_grades
	FROM grade_selection_schedules schedule
	WHERE schedule.grade = ANY(v_grades)
		AND (p_batch_id IS NULL OR schedule.batch_id <> p_batch_id);
	SELECT EXISTS (
		SELECT 1
		FROM grade_selection_schedules schedule
		WHERE schedule.grade = ANY(v_grades)
			AND schedule.opened
			AND (p_batch_id IS NULL OR schedule.batch_id <> p_batch_id)
	)
	INTO v_conflicting_opened;

	IF v_conflicting_opened THEN
		RAISE EXCEPTION 'An open selection window must be edited or manually overridden'
			USING ERRCODE = 'check_violation', CONSTRAINT = 'grade_schedule_opened_conflict';
	END IF;

	IF v_conflicting_grades IS NOT NULL AND NOT p_replace_existing THEN
		RAISE EXCEPTION 'Grades already have a selection schedule: %', array_to_string(v_conflicting_grades, ', ')
			USING ERRCODE = 'unique_violation', CONSTRAINT = 'grade_schedule_exists';
	END IF;

	IF p_batch_id IS NOT NULL THEN
		DELETE FROM grade_selection_schedules
		WHERE batch_id = p_batch_id;
	END IF;
	IF p_replace_existing THEN
		DELETE FROM grade_selection_schedules
		WHERE grade = ANY(v_grades);
	END IF;

	v_batch_id := COALESCE(
		p_batch_id,
		nextval('grade_selection_schedule_batch_id_seq')
	);
	INSERT INTO grade_selection_schedules (
		grade,
		batch_id,
		opens_at,
		closes_at,
		opened
	)
	SELECT
		requested.grade,
		v_batch_id,
		p_opens_at,
		p_closes_at,
		FALSE
	FROM unnest(v_grades) requested(grade);

	-- A future opening must represent a real closed-before-open window. Saving
	-- the schedule closes any currently open selected grades immediately.
	UPDATE grades grade
	SET enabled = FALSE
	WHERE grade.grade = ANY(v_grades)
		AND grade.enabled;

	RETURN v_batch_id;
END;
$$;

CREATE FUNCTION cancel_grade_selection_schedule(p_batch_id BIGINT)
RETURNS BIGINT
LANGUAGE plpgsql
AS $$
DECLARE
	v_deleted BIGINT := 0;
BEGIN
	PERFORM pg_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	);
	DELETE FROM grade_selection_schedules
	WHERE batch_id = p_batch_id;
	GET DIAGNOSTICS v_deleted = ROW_COUNT;
	IF v_deleted = 0 THEN
		RAISE EXCEPTION 'Selection schedule % not found', p_batch_id
			USING ERRCODE = 'no_data_found', CONSTRAINT = 'grade_schedule_batch';
	END IF;
	RETURN v_deleted;
END;
$$;

CREATE FUNCTION apply_due_grade_selection_schedules(
	p_now TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
)
RETURNS TABLE (
	processed_count BIGINT,
	revision BIGINT
)
LANGUAGE plpgsql
AS $$
DECLARE
	v_processed BIGINT := 0;
	v_step BIGINT := 0;
	v_revision BIGINT := 0;
BEGIN
	IF p_now IS NULL THEN
		RAISE EXCEPTION 'A scheduler timestamp is required'
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'grade_schedule_now';
	END IF;

	IF NOT pg_try_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	) THEN
		SELECT state.revision
		INTO v_revision
		FROM selection_access_state state
		WHERE state.singleton = TRUE;
		RETURN QUERY SELECT 0::BIGINT, v_revision;
		RETURN;
	END IF;

	-- If both boundaries passed while the service was offline, closing wins.
	WITH due AS (
		DELETE FROM grade_selection_schedules schedule
		WHERE schedule.closes_at IS NOT NULL
			AND schedule.closes_at <= p_now
		RETURNING schedule.grade
	), changed AS (
		UPDATE grades grade
		SET enabled = FALSE
		FROM due
		WHERE grade.grade = due.grade
		RETURNING grade.grade
	)
	SELECT count(*) INTO v_step FROM due;
	v_processed := v_processed + v_step;

	-- Open-only schedules complete as soon as access is enabled.
	WITH due AS (
		DELETE FROM grade_selection_schedules schedule
		WHERE NOT schedule.opened
			AND schedule.opens_at <= p_now
			AND schedule.closes_at IS NULL
		RETURNING schedule.grade
	), changed AS (
		UPDATE grades grade
		SET enabled = TRUE
		FROM due
		WHERE grade.grade = due.grade
		RETURNING grade.grade
	)
	SELECT count(*) INTO v_step FROM due;
	v_processed := v_processed + v_step;

	-- Windows with a closing time remain present so the same runner can close
	-- them later. opened is operational state and is deleted with the schedule.
	WITH due AS (
		UPDATE grade_selection_schedules schedule
		SET opened = TRUE
		WHERE NOT schedule.opened
			AND schedule.opens_at <= p_now
			AND schedule.closes_at > p_now
		RETURNING schedule.grade
	), changed AS (
		UPDATE grades grade
		SET enabled = TRUE
		FROM due
		WHERE grade.grade = due.grade
		RETURNING grade.grade
	)
	SELECT count(*) INTO v_step FROM due;
	v_processed := v_processed + v_step;

	SELECT state.revision
	INTO v_revision
	FROM selection_access_state state
	WHERE state.singleton = TRUE;
	RETURN QUERY SELECT v_processed, v_revision;
END;
$$;

-- Destructive admin maintenance is kept in one database transaction so that
-- concurrent selection requests cannot race a dependency check. Courses and
-- students must be reset explicitly after selections; this function never
-- performs an implicit cascading reset.
CREATE FUNCTION admin_reset_data(p_scope TEXT)
RETURNS TABLE (
	reset_scope TEXT,
	deleted_count BIGINT,
	closed_grade_count BIGINT
)
LANGUAGE plpgsql
AS $$
DECLARE
	v_deleted BIGINT := 0;
	v_closed_grades BIGINT := 0;
BEGIN
	IF p_scope NOT IN ('selections', 'courses', 'students') THEN
		RAISE EXCEPTION 'Unknown reset scope %', p_scope
			USING ERRCODE = 'invalid_parameter_value', CONSTRAINT = 'reset_scope';
	END IF;

	PERFORM pg_advisory_xact_lock(
		hashtextextended(current_schema() || ':grade-selection-schedules', 0)
	);
	DELETE FROM grade_selection_schedules;

	-- Block choice writes while still allowing the read locks used by the
	-- course-period and foreign-key checks. ACCESS EXCLUSIVE here can deadlock
	-- with a parent-table mutation that already holds its table lock and then
	-- reads choices from a trigger.
	LOCK TABLE choices IN SHARE ROW EXCLUSIVE MODE;

	IF p_scope = 'courses' THEN
		LOCK TABLE courses IN ACCESS EXCLUSIVE MODE;
	ELSIF p_scope = 'students' THEN
		LOCK TABLE students IN ACCESS EXCLUSIVE MODE;
	END IF;

	IF p_scope IN ('courses', 'students')
		AND EXISTS (SELECT 1 FROM choices) THEN
		RAISE EXCEPTION 'Reset selections before resetting %', p_scope
			USING ERRCODE = 'check_violation', CONSTRAINT = 'reset_selections_required';
	END IF;

	UPDATE grades
	SET enabled = FALSE
	WHERE enabled;
	GET DIAGNOSTICS v_closed_grades = ROW_COUNT;

	IF p_scope = 'selections' THEN
		DELETE FROM choices;
	ELSIF p_scope = 'courses' THEN
		DELETE FROM courses;
	ELSE
		DELETE FROM students;
	END IF;
	GET DIAGNOSTICS v_deleted = ROW_COUNT;

	RETURN QUERY SELECT p_scope, v_deleted, v_closed_grades;
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
