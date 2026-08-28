-- Enrollment write functions.
--
-- The student path (self_*) checks the actor-keyed gates,
-- then tolerates no violations;
-- the admin path (place_/remove_enrollments) has no gates,
-- and violations are negotiable through p_accept.
-- The rules themselves live in enrollment_violations;
-- these bodies hold only existence checks, gates, locks,
-- the batch walk, and the raises.
--
-- Lock order, global across every write function:
--   categories, periods, grades, courses, students,
-- each set in ascending id order.
--
-- The rule behind that sequence is: a referenced row is locked before
-- the row that references it. Every table appears before every table
-- holding a foreign key into it — categories, periods and grades are
-- referenced by courses and its association tables; courses and
-- students are referenced by enrollments.
--
-- Half of the locking is invisible in the source, which is why the
-- rule has to be stated as a rule rather than read off the PERFORMs.
-- Inserting a referencing row takes a FOR KEY SHARE lock on the row
-- it names, as part of the foreign key check; and deleting a
-- referenced row takes FOR KEY SHARE on its referrers, as part of
-- proving RESTRICT. So a write that locked a course and then wrote
-- course_periods would be taking courses-then-periods, while
-- DELETE FROM periods takes periods-then-course_periods — and the two
-- deadlock. Parent-before-child is the order that both directions
-- already agree with: a delete naturally holds the parent first, so
-- every write must too.
--
-- Two corollaries are load-bearing:
--   every write that changes students.grade_id must go through
--   upsert_students, and every write that *inserts into*
--   course_periods or course_allowed_grades, or sets
--   courses.category_id, must take the vocabulary locks first —
--   because plain DML cannot take either early enough.
--
--   Inserts, not every touch. Deleting a referencing row takes no
--   lock on the referenced side at all, which is why delete_course,
--   DeleteAllCourses and RenameCourseID reach these tables through a
--   cascade without vocabulary locks and are safe doing it.
--
-- What each lock buys: the course lock stabilizes the capacity count
-- and the enrollee list; the grade lock stabilizes the budget cap;
-- the student lock stabilizes that student's clash and budget facts;
-- the category and period locks buy nothing on their own and exist
-- only to keep the order total.
-- Violations are evaluated after locking,
-- which is what makes the STABLE function's answer
-- authoritative rather than advisory.
--
-- Error vocabulary (class 'YK', application-defined space):
--   YKV01  unaccepted violations.
--          MESSAGE is a human one-liner;
--          DETAIL carries the unaccepted set as a JSON array of
--          {student_id, rule, code, other_course_id, period_id,
--           detail},
--          the machine payload for the confirm dialog.
--          A raise is the only channel out of a refusing function,
--          so the payload rides the error's DETAIL field
--          by declared convention.
--   YKD01  one or more elements of a batch were malformed.
--          DETAIL carries them as a JSON array of
--          {index, id, sqlstate, constraint, field, column, message},
--          so a spreadsheet cell can be pointed at by row and column.
--          An element's sqlstate is normally PostgreSQL's own, but
--          YKD02 there means the batch states that id more than
--          once. It is not an ERRCODE and nothing raises it: no
--          single statement could, because the defect is a fact
--          about the batch and not about any one element.
--          Raised in 0015 and 0017, not here.
--   YKG01  the student's enrollment window is closed.
--   YKG02  the course is invite-only.
--   YKG03  the enrollment is an administrator's placement,
--          which the student may not drop themselves.
--
-- This block is the whole class, not the part of it raised in this
-- file. It listed only V01, G01 and G02 while G03 was being raised
-- twice below it, and the response layer — which was written from this
-- list — did not classify G03 at all, so a student pressing Drop on a
-- placement got a 500. TestEverySQLStateTheSchemaRaisesIsClassified
-- now reads the raises rather than this list, but the list is what a
-- person reads, so it has to be the whole class.
--
-- Existence failures are P0002 (no_data_found);
-- a duplicate enrollment is the primary key's own 23505.
--
-- p_accept semantics (NULL reads as empty, as everywhere):
-- accepted violations that recur proceed silently
-- (they were consented to);
-- accepted violations that no longer exist are ignored
-- (accepts are permissions, not assertions);
-- only violations absent from p_accept block.
-- DETAIL therefore lists exactly the unaccepted set.

-- The lock vocabulary.
--
-- Each takes the rows named, ascending, and raises P0002 if any is
-- missing. NULL is refused (22023) rather than read as "nothing
-- wanted": a NULL array matches nothing and unnests to nothing, so
-- both sides of the existence check come out zero and the call
-- succeeds having locked nothing at all. An empty array really does
-- mean nothing wanted and is allowed, which is what the batch writes
-- pass when a file references no grades.

-- Locks the course rows named, ascending; P0002 if any is missing.
CREATE FUNCTION lock_courses(p_course_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_locked BIGINT;
	v_wanted BIGINT;
BEGIN
	IF p_course_ids IS NULL THEN
		RAISE EXCEPTION 'lock_courses: the id list must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	SELECT count(*) INTO v_locked
	FROM (SELECT 1
		FROM courses c
		WHERE c.id = ANY (p_course_ids)
		ORDER BY c.id
		FOR UPDATE) locked;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_course_ids) AS u;
	IF v_locked <> v_wanted THEN
		RAISE EXCEPTION 'course not found among %', p_course_ids
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- Locks the student rows named, ascending; P0002 if any is missing.
CREATE FUNCTION lock_students(p_student_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_locked BIGINT;
	v_wanted BIGINT;
BEGIN
	IF p_student_ids IS NULL THEN
		RAISE EXCEPTION 'lock_students: the id list must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	SELECT count(*) INTO v_locked
	FROM (SELECT 1
		FROM students s
		WHERE s.id = ANY (p_student_ids)
		ORDER BY s.id
		FOR UPDATE) locked;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_student_ids) AS u;
	IF v_locked <> v_wanted THEN
		RAISE EXCEPTION 'student not found among %', p_student_ids
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- Locks the grade rows named, ascending; P0002 if any is missing.
CREATE FUNCTION lock_grades(p_grade_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_locked BIGINT;
	v_wanted BIGINT;
BEGIN
	IF p_grade_ids IS NULL THEN
		RAISE EXCEPTION 'lock_grades: the id list must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	SELECT count(*) INTO v_locked
	FROM (SELECT 1
		FROM grades g
		WHERE g.id = ANY (p_grade_ids)
		ORDER BY g.id
		FOR UPDATE) locked;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_grade_ids) AS u;
	IF v_locked <> v_wanted THEN
		RAISE EXCEPTION 'grade not found among %', p_grade_ids
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- Locks the period rows named, ascending; P0002 if any is missing.
-- Used only by the declarative reorder, which renumbers every row and
-- would otherwise take them in the caller's array order.
CREATE FUNCTION lock_periods(p_period_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_locked BIGINT;
	v_wanted BIGINT;
BEGIN
	IF p_period_ids IS NULL THEN
		RAISE EXCEPTION 'lock_periods: the id list must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	SELECT count(*) INTO v_locked
	FROM (SELECT 1
		FROM periods p
		WHERE p.id = ANY (p_period_ids)
		ORDER BY p.id
		FOR UPDATE) locked;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_period_ids) AS u;
	IF v_locked <> v_wanted THEN
		RAISE EXCEPTION 'period not found among %', p_period_ids
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- Locks the category rows named, ascending; P0002 if any is missing.
-- Like lock_periods, this buys nothing by itself: it exists so that a
-- write which will reference a category holds it before it holds the
-- course, matching the order a category deletion already takes.
CREATE FUNCTION lock_categories(p_category_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_locked BIGINT;
	v_wanted BIGINT;
BEGIN
	IF p_category_ids IS NULL THEN
		RAISE EXCEPTION 'lock_categories: the id list must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	SELECT count(*) INTO v_locked
	FROM (SELECT 1
		FROM categories c
		WHERE c.id = ANY (p_category_ids)
		ORDER BY c.id
		FOR UPDATE) locked;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_category_ids) AS u;
	IF v_locked <> v_wanted THEN
		RAISE EXCEPTION 'category not found among %', p_category_ids
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- Raises YKV01 carrying p_unaccepted (already JSON) if non-empty.
CREATE FUNCTION raise_unaccepted(p_unaccepted JSONB)
RETURNS void
LANGUAGE plpgsql
IMMUTABLE
AS $$
BEGIN
	IF p_unaccepted IS NOT NULL AND jsonb_array_length(p_unaccepted) > 0 THEN
		RAISE EXCEPTION '% unaccepted violation(s)',
			jsonb_array_length(p_unaccepted)
			USING ERRCODE = 'YKV01', DETAIL = p_unaccepted::TEXT;
	END IF;
END;
$$;

-- The violations of one prospective enrollment, as JSONB,
-- excluding codes in p_accept.
-- Factored out so every write path builds the payload identically.
CREATE FUNCTION unaccepted_violations(
	p_student_id localpart,
	p_course_id entity_id,
	p_counts_toward_budget BOOLEAN,
	p_disregard_course_ids TEXT[],
	p_accept TEXT[]
)
RETURNS JSONB
LANGUAGE sql
STABLE
BEGIN ATOMIC
SELECT COALESCE(jsonb_agg(jsonb_build_object(
		'student_id', p_student_id,
		'rule', v.rule,
		'code', v.code,
		'other_course_id', v.other_course_id,
		'period_id', v.period_id,
		'detail', v.detail)), '[]'::JSONB)
FROM enrollment_violations(p_student_id, p_course_id,
	p_counts_toward_budget, p_disregard_course_ids) v
WHERE NOT (v.code = ANY (COALESCE(p_accept, '{}')));
END;

-- Re-judging helpers.
--
-- Every write that changes a rule's inputs applies its change and
-- then re-judges the enrollments the change could have broken,
-- scoped to the rules whose inputs actually moved.
-- There are exactly two axes to re-judge along —
-- a course's enrollees, and a student's enrollments —
-- plus budget, which is a per-student aggregate rather than a fact
-- about any one enrollment.
-- These three functions are that whole vocabulary;
-- write functions choose an axis and a rule list, and never
-- restate the walk.
--
-- All three are LANGUAGE sql: the walk is a join, not control flow,
-- so it belongs on the rung PostgreSQL checks at definition time.
-- Results are ordered deterministically (subject, then the order
-- enrollment_violations produced them) so payloads are stable
-- and testable.

-- One course's enrollees, judged on the named rules.
-- 'capacity' does not belong in p_rules:
-- capacity is a course-level fact, reported once for the course,
-- never once per enrollee.
CREATE FUNCTION course_enrollee_violations(
	p_course_id entity_id,
	p_rules TEXT[],
	p_accept TEXT[]
)
RETURNS JSONB
LANGUAGE sql
STABLE
BEGIN ATOMIC
SELECT COALESCE(jsonb_agg(a.u ORDER BY e.student_id, a.ord), '[]'::JSONB)
FROM enrollments e
CROSS JOIN LATERAL jsonb_array_elements(unaccepted_violations(
	e.student_id, p_course_id, e.counts_toward_budget,
	'{}', p_accept)) WITH ORDINALITY AS a (u, ord)
WHERE e.course_id = p_course_id
	AND a.u->>'rule' = ANY (p_rules);
END;

-- One student's enrollments, judged on the named rules.
-- The mirror image of the above, for changes to the student
-- (grade, legal sex) rather than to a course.
CREATE FUNCTION student_enrollment_violations(
	p_student_id localpart,
	p_rules TEXT[],
	p_accept TEXT[]
)
RETURNS JSONB
LANGUAGE sql
STABLE
BEGIN ATOMIC
SELECT COALESCE(jsonb_agg(a.u ORDER BY e.course_id, a.ord), '[]'::JSONB)
FROM enrollments e
CROSS JOIN LATERAL jsonb_array_elements(unaccepted_violations(
	p_student_id, e.course_id, e.counts_toward_budget,
	'{}', p_accept)) WITH ORDINALITY AS a (u, ord)
WHERE e.student_id = p_student_id
	AND a.u->>'rule' = ANY (p_rules);
END;

-- One student's budget, judged once.
-- Budget is the sum over all of a student's charging enrollments,
-- so judging per enrollment would report the same fact repeatedly.
-- The rule is evaluated through an arbitrary one of those
-- enrollments (the lowest course id): enrollment_violations sums
-- them all regardless of which is nominally the subject, and the
-- code it produces names the student alone.
-- A student with nothing charging has no budget to exceed,
-- and the HAVING makes the subquery empty rather than passing a
-- NULL course into the rule.
CREATE FUNCTION student_budget_violation(
	p_student_id localpart,
	p_accept TEXT[]
)
RETURNS JSONB
LANGUAGE sql
STABLE
BEGIN ATOMIC
SELECT COALESCE(jsonb_agg(a.u ORDER BY a.ord), '[]'::JSONB)
FROM (SELECT min(e.course_id) AS course_id
	FROM enrollments e
	WHERE e.student_id = p_student_id
		AND e.counts_toward_budget
	HAVING count(*) > 0) c
CROSS JOIN LATERAL jsonb_array_elements(unaccepted_violations(
	p_student_id, c.course_id, TRUE,
	'{}', p_accept)) WITH ORDINALITY AS a (u, ord)
WHERE a.u->>'rule' = 'budget';
END;

-- A student enrolls themselves.
-- Gates: their grade's window must be open (YKG01)
-- and the course must not be invite-only (YKG02).
-- Then no violations are tolerated: students accept nothing.
-- Always creates a droppable, charging row.
CREATE FUNCTION self_enroll(
	p_student_id localpart,
	p_course_id entity_id
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_invite_only BOOLEAN;
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	PERFORM lock_students(ARRAY[p_student_id]);

	SELECT c.invite_only INTO v_invite_only
	FROM courses c WHERE c.id = p_course_id;
	IF v_invite_only THEN
		RAISE EXCEPTION 'course % is invite-only', p_course_id
			USING ERRCODE = 'YKG02';
	END IF;

	IF NOT EXISTS (SELECT 1
		FROM v_grades g
		JOIN students s ON s.grade_id = g.id
		WHERE s.id = p_student_id AND g.is_open)
	THEN
		RAISE EXCEPTION 'enrollment window is closed for student %',
			p_student_id
			USING ERRCODE = 'YKG01';
	END IF;

	PERFORM raise_unaccepted(unaccepted_violations(
		p_student_id, p_course_id, TRUE, '{}', '{}'));

	INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
	VALUES (p_student_id, p_course_id, TRUE, TRUE);
END;
$$;

-- A student drops their own enrollment.
-- Gates: window open (YKG01);
-- the row must exist (P0002) and be student_droppable
-- (YKG03: not droppable).
CREATE FUNCTION self_drop(
	p_student_id localpart,
	p_course_id entity_id
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_droppable BOOLEAN;
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	PERFORM lock_students(ARRAY[p_student_id]);

	IF NOT EXISTS (SELECT 1
		FROM v_grades g
		JOIN students s ON s.grade_id = g.id
		WHERE s.id = p_student_id AND g.is_open)
	THEN
		RAISE EXCEPTION 'enrollment window is closed for student %',
			p_student_id
			USING ERRCODE = 'YKG01';
	END IF;

	SELECT e.student_droppable INTO v_droppable
	FROM enrollments e
	WHERE e.student_id = p_student_id AND e.course_id = p_course_id;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'no enrollment of % in %',
			p_student_id, p_course_id
			USING ERRCODE = 'P0002';
	END IF;
	IF NOT v_droppable THEN
		RAISE EXCEPTION 'enrollment of % in % is not droppable',
			p_student_id, p_course_id
			USING ERRCODE = 'YKG03';
	END IF;

	DELETE FROM enrollments
	WHERE student_id = p_student_id AND course_id = p_course_id;
END;
$$;

-- A student atomically replaces enrollments:
-- every old course dropped and the new one added, or nothing.
-- Old rows must all exist (P0002) and be droppable (YKG03);
-- the replacement is judged with the old courses disregarded,
-- so swapping between clashing alternatives works.
CREATE FUNCTION self_swap(
	p_student_id localpart,
	p_old_course_ids TEXT[],
	p_course_id entity_id
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_invite_only BOOLEAN;
	v_bad TEXT;
BEGIN
	PERFORM lock_courses(p_old_course_ids || p_course_id::TEXT);
	PERFORM lock_students(ARRAY[p_student_id]);

	SELECT c.invite_only INTO v_invite_only
	FROM courses c WHERE c.id = p_course_id;
	IF v_invite_only THEN
		RAISE EXCEPTION 'course % is invite-only', p_course_id
			USING ERRCODE = 'YKG02';
	END IF;

	IF NOT EXISTS (SELECT 1
		FROM v_grades g
		JOIN students s ON s.grade_id = g.id
		WHERE s.id = p_student_id AND g.is_open)
	THEN
		RAISE EXCEPTION 'enrollment window is closed for student %',
			p_student_id
			USING ERRCODE = 'YKG01';
	END IF;

	SELECT u INTO v_bad
	FROM unnest(p_old_course_ids) AS u
	WHERE NOT EXISTS (SELECT 1
		FROM enrollments e
		WHERE e.student_id = p_student_id
			AND e.course_id = u
			AND e.student_droppable)
	LIMIT 1;
	IF v_bad IS NOT NULL THEN
		IF EXISTS (SELECT 1 FROM enrollments e
			WHERE e.student_id = p_student_id AND e.course_id = v_bad)
		THEN
			RAISE EXCEPTION 'enrollment of % in % is not droppable',
				p_student_id, v_bad
				USING ERRCODE = 'YKG03';
		END IF;
		RAISE EXCEPTION 'no enrollment of % in %', p_student_id, v_bad
			USING ERRCODE = 'P0002';
	END IF;

	PERFORM raise_unaccepted(unaccepted_violations(
		p_student_id, p_course_id, TRUE, p_old_course_ids, '{}'));

	DELETE FROM enrollments
	WHERE student_id = p_student_id
		AND course_id = ANY (p_old_course_ids);
	INSERT INTO enrollments
		(student_id, course_id, student_droppable, counts_toward_budget)
	VALUES (p_student_id, p_course_id, TRUE, TRUE);
END;
$$;

-- An administrator places a set of students into one course.
-- No gates; the policy bits apply uniformly to the batch.
-- Students are walked in the caller's order,
-- each judged against the world as it stands,
-- including rows inserted for earlier elements,
-- so seat consumption within the batch is caught
-- (a clash cannot be: one call places into one course,
-- and a student named twice is the primary key's 23505);
-- with contested seats, earlier elements win.
-- Rows are inserted even when violating:
-- the final raise aborts the whole transaction,
-- so either every row stands or none does.
CREATE FUNCTION place_enrollments(
	p_course_id entity_id,
	p_student_ids TEXT[],
	p_student_droppable BOOLEAN,
	p_counts_toward_budget BOOLEAN,
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_student TEXT;
	v_unaccepted JSONB := '[]'::JSONB;
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	PERFORM lock_students(p_student_ids);

	FOREACH v_student IN ARRAY p_student_ids LOOP
		v_unaccepted := v_unaccepted || unaccepted_violations(
			v_student, p_course_id, p_counts_toward_budget,
			'{}', p_accept);
		INSERT INTO enrollments
			(student_id, course_id,
			 student_droppable, counts_toward_budget)
		VALUES (v_student, p_course_id,
			p_student_droppable, p_counts_toward_budget);
	END LOOP;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;

-- An administrator removes enrollments from one course.
-- No gates and no accept parameter:
-- removal cannot violate the acceptable set,
-- and the droppability bit binds students only.
-- Every named enrollment must exist (P0002).
CREATE FUNCTION remove_enrollments(
	p_course_id entity_id,
	p_student_ids TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_deleted BIGINT;
	v_wanted BIGINT;
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	PERFORM lock_students(p_student_ids);

	DELETE FROM enrollments
	WHERE course_id = p_course_id
		AND student_id = ANY (p_student_ids);
	GET DIAGNOSTICS v_deleted = ROW_COUNT;
	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_student_ids) AS u;
	IF v_deleted <> v_wanted THEN
		RAISE EXCEPTION 'not every named enrollment in % exists',
			p_course_id
			USING ERRCODE = 'P0002';
	END IF;
END;
$$;

-- An administrator changes the policy of enrollments that already
-- exist, in one course, without disturbing the seats.
--
-- The alternative is remove-then-place, which is two operations for
-- one intention: it gives up the seat in between, so a full course
-- can lose it to somebody else, and a refusal on the second half
-- leaves the student out of a course they were in.
--
-- The two bits differ in what they can break, and the accept list
-- reflects only the one that can:
--   student_droppable     binds students, is no rule's input,
--                         and re-judges nobody
--   counts_toward_budget  is the budget rule's input, so turning it
--                         on can put a student over their cap
-- Nothing else moves, so no other rule is re-judged: the same
-- students hold the same seats in the same periods as before.
--
-- Every named enrollment must already exist (P0002). This is not an
-- upsert: placing and re-policying are different intentions, and
-- silently creating a row here would hide a typo in a student id.
CREATE FUNCTION set_enrollment_policy(
	p_course_id entity_id,
	p_student_ids TEXT[],
	p_student_droppable BOOLEAN,
	p_counts_toward_budget BOOLEAN,
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_student TEXT;
	v_present BIGINT;
	v_wanted BIGINT;
	v_moved TEXT[];
	v_unaccepted JSONB := '[]'::JSONB;
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	PERFORM lock_students(p_student_ids);

	-- Existence is checked against what is there, not against what
	-- the update touched. Those used to be the same number, which is
	-- why the update could report both facts at once; now that a row
	-- already holding the requested policy is left alone, an
	-- unchanged row would look like a missing one.
	SELECT count(*) INTO v_present
	FROM enrollments e
	WHERE e.course_id = p_course_id
		AND e.student_id = ANY (p_student_ids);

	SELECT count(DISTINCT u) INTO v_wanted
	FROM unnest(p_student_ids) AS u;

	IF v_present <> v_wanted THEN
		RAISE EXCEPTION 'not every named enrollment in % exists',
			p_course_id
			USING ERRCODE = 'P0002';
	END IF;

	-- Whose budget bit is about to turn on.
	--
	-- Read before the update, because the transition is what matters
	-- and only the old value can tell us there was one. Postgres 18
	-- would allow RETURNING OLD; on 17 the set has to be taken first.
	--
	-- Scoping matters twice over. The write is declarative, so
	-- re-saving the same policy must be a no-op; and per-write rule
	-- scoping says a rule is judged only when the write moved one of
	-- that rule's inputs. Judging every row the statement touched
	-- conflates both with a third thing — the droppable bit, which is
	-- no rule's input at all. Flipping only that on a student whose
	-- enrollments were already over an accepted cap would re-run the
	-- budget rule and refuse a write that cannot affect it.
	--
	-- Only FALSE -> TRUE can tighten the sum. Turning the bit off
	-- relaxes it, and leaving it alone changes nothing.
	SELECT array_agg(DISTINCT e.student_id::TEXT)
	INTO v_moved
	FROM enrollments e
	WHERE p_counts_toward_budget
		AND e.course_id = p_course_id
		AND e.student_id = ANY (p_student_ids)
		AND NOT e.counts_toward_budget;

	UPDATE enrollments e
	SET student_droppable = p_student_droppable,
		counts_toward_budget = p_counts_toward_budget
	WHERE e.course_id = p_course_id
		AND e.student_id = ANY (p_student_ids)
		AND (e.student_droppable, e.counts_toward_budget)
			IS DISTINCT FROM
			(p_student_droppable, p_counts_toward_budget);

	FOREACH v_student IN ARRAY COALESCE(v_moved, ARRAY[]::TEXT[]) LOOP
		v_unaccepted := v_unaccepted
			|| student_budget_violation(v_student, p_accept);
	END LOOP;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;
