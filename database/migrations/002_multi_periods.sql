BEGIN;

-- This is the only production upgrade path from the original schema. Lock the
-- version row so concurrent migration attempts serialize, then lock every
-- legacy table whose keys or period values are inspected or replaced.
DO $$
DECLARE
	v_version BIGINT;
BEGIN
	SELECT version
	INTO v_version
	FROM schema_version
	WHERE singleton = TRUE
	FOR UPDATE;

	IF NOT FOUND OR v_version <> 1 THEN
		RAISE EXCEPTION 'Migration 002 requires schema version 1, found %',
			COALESCE(v_version::TEXT, 'no version row');
	END IF;
END;
$$;

LOCK TABLE choices, courses, periods, grades IN ACCESS EXCLUSIVE MODE;

-- Selection-access schedules are operational state, not audit history. Rows
-- created together share a batch_id so the admin UI can present them as one
-- schedule while still enforcing at most one schedule per grade.
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

-- Application instances poll this revision so a schedule applied by one
-- instance still invalidates student data cached behind every other instance.
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

CREATE TEMPORARY TABLE fixed_period_aliases (
	alias_id TEXT NOT NULL,
	canonical_id TEXT NOT NULL,
	ordinal SMALLINT NOT NULL,
	PRIMARY KEY (alias_id, canonical_id)
) ON COMMIT DROP;

-- The original version 1 installations may use short English aliases. Canonical v2 names are also
-- included so partially hand-normalized databases migrate without special
-- handling. No fuzzy or case-insensitive guessing is performed.
INSERT INTO fixed_period_aliases (alias_id, canonical_id, ordinal) VALUES
	('Mon 1', 'Monday CCA 1', 1),
	('Monday CCA 1', 'Monday CCA 1', 1),
	('Mon 2', 'Monday CCA 2', 2),
	('Monday CCA 2', 'Monday CCA 2', 2),
	('Mon 3', 'Monday CCA 3', 3),
	('Monday CCA 3', 'Monday CCA 3', 3),
	('Mon 4', 'Monday CCA 4', 4),
	('Monday CCA 4', 'Monday CCA 4', 4),
	('Tue 1', 'Tuesday CCA 1', 5),
	('Tuesday CCA 1', 'Tuesday CCA 1', 5),
	('Tue 2', 'Tuesday CCA 2', 6),
	('Tuesday CCA 2', 'Tuesday CCA 2', 6),
	('Tue 3', 'Tuesday CCA 3', 7),
	('Tuesday CCA 3', 'Tuesday CCA 3', 7),
	('Tue 4', 'Tuesday CCA 4', 8),
	('Tuesday CCA 4', 'Tuesday CCA 4', 8),
	('Wed 1', 'Wednesday CCA 1', 9),
	('Wednesday CCA 1', 'Wednesday CCA 1', 9),
	('Wed 2', 'Wednesday CCA 2', 10),
	('Wednesday CCA 2', 'Wednesday CCA 2', 10),
	('Wed 3', 'Wednesday CCA 3', 11),
	('Wednesday CCA 3', 'Wednesday CCA 3', 11),
	('Wed 4', 'Wednesday CCA 4', 12),
	('Wednesday CCA 4', 'Wednesday CCA 4', 12),
	('Thu 1', 'Thursday CCA 1', 13),
	('Thursday CCA 1', 'Thursday CCA 1', 13),
	('Thu 2', 'Thursday CCA 2', 14),
	('Thursday CCA 2', 'Thursday CCA 2', 14),
	('Thu 3', 'Thursday CCA 3', 15),
	('Thursday CCA 3', 'Thursday CCA 3', 15),
	('Thu 4', 'Thursday CCA 4', 16),
	('Thursday CCA 4', 'Thursday CCA 4', 16),
	('MW1', 'Monday CCA 1', 1),
	('MW1', 'Wednesday CCA 1', 9),
	('Monday/Wednesday CCA 1', 'Monday CCA 1', 1),
	('Monday/Wednesday CCA 1', 'Wednesday CCA 1', 9),
	('MW2', 'Monday CCA 2', 2),
	('MW2', 'Wednesday CCA 2', 10),
	('Monday/Wednesday CCA 2', 'Monday CCA 2', 2),
	('Monday/Wednesday CCA 2', 'Wednesday CCA 2', 10),
	('MW3', 'Monday CCA 3', 3),
	('MW3', 'Wednesday CCA 3', 11),
	('Monday/Wednesday CCA 3', 'Monday CCA 3', 3),
	('Monday/Wednesday CCA 3', 'Wednesday CCA 3', 11),
	('MW4', 'Monday CCA 4', 4),
	('MW4', 'Wednesday CCA 4', 12),
	('Monday/Wednesday CCA 4', 'Monday CCA 4', 4),
	('Monday/Wednesday CCA 4', 'Wednesday CCA 4', 12),
	('TT1', 'Tuesday CCA 1', 5),
	('TT1', 'Thursday CCA 1', 13),
	('Tuesday/Thursday CCA 1', 'Tuesday CCA 1', 5),
	('Tuesday/Thursday CCA 1', 'Thursday CCA 1', 13),
	('TT2', 'Tuesday CCA 2', 6),
	('TT2', 'Thursday CCA 2', 14),
	('Tuesday/Thursday CCA 2', 'Tuesday CCA 2', 6),
	('Tuesday/Thursday CCA 2', 'Thursday CCA 2', 14),
	('TT3', 'Tuesday CCA 3', 7),
	('TT3', 'Thursday CCA 3', 15),
	('Tuesday/Thursday CCA 3', 'Tuesday CCA 3', 7),
	('Tuesday/Thursday CCA 3', 'Thursday CCA 3', 15),
	('TT4', 'Tuesday CCA 4', 8),
	('TT4', 'Thursday CCA 4', 16),
	('Tuesday/Thursday CCA 4', 'Tuesday CCA 4', 8),
	('Tuesday/Thursday CCA 4', 'Thursday CCA 4', 16);

-- Unsupported unoccupied legacy labels can be discarded safely. An unsupported
-- occupied label cannot: abort with all offending IDs so an administrator can
-- reassign those courses explicitly before retrying.
DO $$
DECLARE
	v_unsupported TEXT;
BEGIN
	SELECT string_agg(quote_literal(unsupported.period_id), ', ' ORDER BY unsupported.period_id)
	INTO v_unsupported
	FROM (
		SELECT DISTINCT c.period AS period_id
		FROM courses c
		LEFT JOIN fixed_period_aliases aliases ON aliases.alias_id = c.period
		WHERE aliases.alias_id IS NULL
	) unsupported;

	IF v_unsupported IS NOT NULL THEN
		RAISE EXCEPTION 'Migration 002 cannot map occupied period(s): %. Reassign them to Mon/Tue/Wed/Thu 1-4 before retrying.',
			v_unsupported
			USING ERRCODE = 'check_violation', CONSTRAINT = 'periods_unsupported';
	END IF;
END;
$$;

-- Expand every legacy course period into its final canonical slots before
-- touching permanent schema objects. One legacy compound label may produce two
-- rows; DISTINCT also makes alias expansion idempotent within this transaction.
CREATE TEMPORARY TABLE migrated_course_periods ON COMMIT DROP AS
SELECT DISTINCT c.id AS course_id, aliases.canonical_id AS period_id
FROM courses c
JOIN fixed_period_aliases aliases ON aliases.alias_id = c.period;

-- A v1 choice has only the legacy course period. In v2, course periods are
-- alternatives and every choice must point at one concrete fixed slot. When a
-- legacy compound alias expands to more than one slot, preserve the choice in
-- the earliest canonical slot; administrators can move it after migration.
CREATE TEMPORARY TABLE migrated_choices ON COMMIT DROP AS
SELECT
	ch.student_id,
	ch.course_id,
	(
		SELECT aliases.canonical_id
		FROM fixed_period_aliases aliases
		WHERE aliases.alias_id = ch.period
		ORDER BY aliases.ordinal
		LIMIT 1
	) AS period_id
FROM choices ch;

DO $$
DECLARE
	v_unscheduled TEXT;
BEGIN
	SELECT string_agg(quote_literal(c.id), ', ' ORDER BY c.id)
	INTO v_unscheduled
	FROM courses c
	WHERE NOT EXISTS (
		SELECT 1
		FROM migrated_course_periods cp
		WHERE cp.course_id = c.id
	);

	IF v_unscheduled IS NOT NULL THEN
		RAISE EXCEPTION 'Migration 002 found course(s) without supported periods: %', v_unscheduled
			USING ERRCODE = 'check_violation', CONSTRAINT = 'course_requires_period';
	END IF;
END;
$$;

-- Version 1 considered different text labels to be different periods. Detect
-- expansions/collapses that would place two preserved choices into one final
-- slot; the new row trigger cannot retroactively validate existing rows.
DO $$
DECLARE
	v_student_id BIGINT;
	v_period_id TEXT;
	v_course_ids TEXT;
BEGIN
	SELECT
		ch.student_id,
		ch.period_id,
		string_agg(ch.course_id, ', ' ORDER BY ch.course_id)
	INTO v_student_id, v_period_id, v_course_ids
	FROM migrated_choices ch
	GROUP BY ch.student_id, ch.period_id
	HAVING count(DISTINCT ch.course_id) > 1
	ORDER BY ch.student_id, ch.period_id
	LIMIT 1;

	IF FOUND THEN
		RAISE EXCEPTION 'Migration 002 would double-book student % in % across courses %',
			v_student_id, v_period_id, v_course_ids
			USING ERRCODE = 'check_violation', CONSTRAINT = 'choices_period_conflict';
	END IF;
END;
$$;

-- Replace the legacy composite course/period choice key directly with the final
-- multi-period representation. No flexible-period schema is created in between.
DROP VIEW v_export_selections;
DROP TRIGGER trg_choices_constraints ON choices;
DROP FUNCTION enforce_choice_constraints();
DROP FUNCTION delete_choice(BIGINT, TEXT);
DROP FUNCTION new_selection(BIGINT, TEXT, selection_type);
DROP INDEX IF EXISTS idx_choices_course_period;
DROP INDEX IF EXISTS idx_courses_period;

ALTER TABLE choices DROP CONSTRAINT choices_course_id_period_fkey;
ALTER TABLE choices DROP CONSTRAINT choices_pkey;
ALTER TABLE choices DROP CONSTRAINT choices_student_id_course_id_key;
ALTER TABLE choices ADD COLUMN period_id TEXT;
UPDATE choices ch
SET period_id = migrated.period_id
FROM migrated_choices migrated
WHERE migrated.student_id = ch.student_id
	AND migrated.course_id = ch.course_id;
ALTER TABLE choices ALTER COLUMN period_id SET NOT NULL;
ALTER TABLE choices DROP COLUMN period;
ALTER TABLE choices ADD PRIMARY KEY (student_id, course_id);

ALTER TABLE courses DROP CONSTRAINT courses_id_period_key;
ALTER TABLE courses DROP CONSTRAINT courses_period_fkey;
ALTER TABLE courses DROP COLUMN period;

ALTER TABLE periods ADD COLUMN ordinal SMALLINT;
DELETE FROM periods;
INSERT INTO periods (id, ordinal)
SELECT canonical_id, ordinal
FROM fixed_period_aliases
WHERE alias_id = canonical_id
ORDER BY ordinal;
ALTER TABLE periods ALTER COLUMN ordinal SET NOT NULL;
ALTER TABLE periods
	ADD CONSTRAINT periods_ordinal_key UNIQUE (ordinal),
	ADD CONSTRAINT periods_ordinal_check CHECK (ordinal BETWEEN 1 AND 16);

CREATE TABLE course_periods (
	course_id TEXT NOT NULL REFERENCES courses(id) ON UPDATE CASCADE ON DELETE CASCADE,
	period_id TEXT NOT NULL REFERENCES periods(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
	PRIMARY KEY (course_id, period_id)
);
INSERT INTO course_periods (course_id, period_id)
SELECT course_id, period_id
FROM migrated_course_periods;

ALTER TABLE choices
	ADD CONSTRAINT choices_period_conflict UNIQUE (student_id, period_id),
	ADD CONSTRAINT choices_course_period_fkey
		FOREIGN KEY (course_id, period_id)
		REFERENCES course_periods(course_id, period_id)
			ON UPDATE CASCADE ON DELETE RESTRICT;

-- Capacity counts and their WebSocket ordering revisions are authoritative
-- database state. Seed them from the migrated choices before installing the
-- maintenance triggers.
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

-- Enforce all final selection rules under deterministic student-then-course row locks.
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

	-- Lock every affected course after the student and in lexical order.
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

	-- A course may offer multiple fixed slots, but a choice selects exactly one.
	-- Timetable conflicts are hard constraints for every selection type.
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

	-- Capacity comes from transactionally maintained PostgreSQL state.
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

-- Destructive admin maintenance is kept in one database transaction so that
-- concurrent selection requests cannot race a dependency check. Courses and
-- students must be reset explicitly after selections; this function never
-- performs an implicit cascading reset.
-- Manual access changes and scheduled changes share one database lock. This
-- makes the database the source of truth without requiring pg_cron.
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

UPDATE schema_version
SET version = 2
WHERE singleton = TRUE;

COMMIT;
