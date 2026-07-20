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

LOCK TABLE choices, courses, periods IN ACCESS EXCLUSIVE MODE;

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

	-- The lock order for choice mutations is always student, then course.
	-- Course schedule mutations lock only the course and are rejected once the
	-- course has choices, so the two workflows cannot race or deadlock.
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

-- A selected course's timetable is immutable. This deliberately conservative
-- rule keeps schedule edits race-free: administrators must clear/reconcile the
-- course's choices before changing its occupied periods.
CREATE FUNCTION enforce_course_period_immutability()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	v_course_id TEXT;
BEGIN
	IF TG_OP = 'DELETE' THEN
		v_course_id := OLD.course_id;
	ELSIF TG_OP = 'UPDATE' AND OLD.course_id IS DISTINCT FROM NEW.course_id THEN
		RAISE EXCEPTION 'Move a timetable period by deleting and inserting it in one transaction'
			USING ERRCODE = 'check_violation';
	ELSE
		v_course_id := NEW.course_id;
	END IF;

	PERFORM 1
	FROM courses c
	WHERE c.id = v_course_id
	FOR UPDATE;

	IF EXISTS (SELECT 1 FROM choices ch WHERE ch.course_id = v_course_id) THEN
		RAISE EXCEPTION 'Cannot change timetable periods for selected course %', v_course_id
			USING ERRCODE = 'check_violation', CONSTRAINT = 'course_periods_immutable';
	END IF;

	IF TG_OP = 'DELETE' THEN
		RETURN OLD;
	END IF;
	RETURN NEW;
END;
$$;
CREATE TRIGGER trg_course_periods_immutability
BEFORE INSERT OR UPDATE OR DELETE ON course_periods
FOR EACH ROW
EXECUTE FUNCTION enforce_course_period_immutability();

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
