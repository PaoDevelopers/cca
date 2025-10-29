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
INSERT INTO schema_version (version) VALUES (1);

-- This is the 'gender' field from PowerSchool, but it is more accurately
-- described as legal sex.
CREATE TYPE legal_sex AS ENUM ('F', 'M', 'X');

-- The only difference 'invite' has on top of 'no', is that the schema will
-- allow inserting 'invite's that don't satisfy typical constraints such as
-- legal sex restrictions, maximum member counts, or year group restrictions. A
-- selection type of 'force' means that the student would not be allowed to
-- remove the selection. Students are only able to add selections of type 'no';
-- the others may only be added by administrators.
CREATE TYPE selection_type AS ENUM ('no', 'invite', 'force');

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
	-- of choices with invitation_type="no" that they have exceeds the
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

-- Course periods such as 'MW1', 'TT3', etc.
CREATE TABLE periods (
	id TEXT PRIMARY KEY CHECK (btrim(id) <> '')
);

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
	period TEXT NOT NULL REFERENCES periods(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
	max_students BIGINT NOT NULL CHECK (max_students >= 0),
	membership membership_type NOT NULL DEFAULT 'free',
	teacher TEXT NOT NULL,
	location TEXT NOT NULL,
	category_id TEXT NOT NULL REFERENCES categories(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
	-- This UNIQUE is intentionally kept even though id is PK, so the
	-- composite FK from choices can ensure stored period matches the
	-- course's period.
	UNIQUE (id, period)
);

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
	period TEXT NOT NULL,
	selection_type selection_type NOT NULL DEFAULT 'no',
	PRIMARY KEY (student_id, period),
	UNIQUE (student_id, course_id),
	-- This is the reason for the UNIQUE on courses.
	FOREIGN KEY (course_id, period) REFERENCES courses(id, period) ON UPDATE CASCADE ON DELETE RESTRICT
);

-- Enforce legal_sex/grade/membership/capacity/selection_window only when
-- selection_type = 'no'. Invites/forces bypass these checks by design.
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
BEGIN
	-- Gate: only act when the resulting row is a normal selection
	IF NOT (
		NEW.selection_type = 'no' AND
		(
			TG_OP = 'INSERT'
			OR OLD.selection_type IS DISTINCT FROM 'no'
			OR OLD.course_id IS DISTINCT FROM NEW.course_id
		)
	) THEN
		RETURN NEW;
	END IF;

	-- Student attributes
	SELECT s.grade, s.legal_sex
	INTO v_student_grade, v_student_legal_sex
	FROM students s
	WHERE s.id = NEW.student_id;

	IF v_student_grade IS NULL THEN
		RAISE EXCEPTION 'Student % not found', NEW.student_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	-- Lock course row once; get all needed fields
	SELECT c.max_students, c.membership
	INTO v_max, v_membership
	FROM courses c
	WHERE c.id = NEW.course_id
	FOR UPDATE;

	IF v_max IS NULL THEN
		RAISE EXCEPTION 'Course % not found', NEW.course_id
			USING ERRCODE = 'foreign_key_violation';
	END IF;

	-- Membership (invite-only needs an invitation)
	IF v_membership = 'invite_only' THEN
		RAISE EXCEPTION 'Course % is invite-only; invitation required', NEW.course_id
			USING ERRCODE = 'check_violation';
	END IF;

	-- Legal sex restriction
	SELECT
		EXISTS (SELECT 1 FROM course_allowed_legal_sexes s WHERE s.course_id = NEW.course_id),
		EXISTS (SELECT 1 FROM course_allowed_legal_sexes s WHERE s.course_id = NEW.course_id AND s.legal_sex = v_student_legal_sex)
	INTO v_has_legal_sex_list, v_legal_sex_allowed;

	IF v_has_legal_sex_list AND NOT v_legal_sex_allowed THEN
		RAISE EXCEPTION 'Student % legal sex % not allowed for course %',
			NEW.student_id, v_student_legal_sex, NEW.course_id
			USING ERRCODE = 'check_violation';
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
			USING ERRCODE = 'check_violation';
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
			USING ERRCODE = 'check_violation';
	END IF;

	-- Own selections cap (count only selections with selection_type = 'no')
	SELECT COUNT(*)::bigint
	INTO v_student_no_count
	FROM choices
	WHERE student_id = NEW.student_id
		AND selection_type = 'no';

	IF TG_OP = 'INSERT' OR OLD.selection_type IS DISTINCT FROM 'no' THEN
		v_student_no_count := v_student_no_count + 1;

		IF v_student_no_count > v_max_own_choices THEN
			RAISE EXCEPTION 'Student % cannot exceed % own selections for grade %',
				NEW.student_id, v_max_own_choices, v_student_grade
				USING ERRCODE = 'check_violation';
		END IF;
	END IF;

	-- Capacity (after locking the course row)
	SELECT COUNT(*)::bigint
	INTO v_count
	FROM choices
	WHERE course_id = NEW.course_id;

	-- Neutralize self-count on UPDATE within same course
	IF TG_OP = 'UPDATE' AND OLD.course_id = NEW.course_id THEN
		v_count := v_count - 1;
	END IF;

	IF v_count >= v_max THEN
		RAISE EXCEPTION 'Course % is at capacity (% >= %)', NEW.course_id, v_count, v_max
			USING ERRCODE = 'check_violation';
	END IF;

	RETURN NEW;
END
$$;
CREATE TRIGGER trg_choices_constraints
BEFORE INSERT OR UPDATE OF course_id, selection_type ON choices
FOR EACH ROW
EXECUTE FUNCTION enforce_choice_constraints();




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
			USING ERRCODE = 'check_violation';
	END IF;

	IF v_selection_type = 'force' THEN
		RAISE EXCEPTION 'Cannot delete forced selection for student % and course %',
			p_student_id, p_course_id
			USING ERRCODE = 'check_violation';
	END IF;

	DELETE FROM choices
	WHERE student_id = p_student_id AND course_id = p_course_id;
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
	c.period AS period,
	ch.selection_type AS selection_type
FROM choices ch
JOIN students s ON s.id = ch.student_id
JOIN courses c ON c.id = ch.course_id
ORDER BY s.id, c.period, c.id;

-- Indxes

-- TODO for after I write the queries
