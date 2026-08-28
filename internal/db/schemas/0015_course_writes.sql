-- Course writes.
--
-- A course is created and edited as a whole.
-- Its attributes, its periods, its restrictions and its capacity
-- are one form and one save button, so they are one call and one
-- transaction: an administrator cannot end up with the name applied
-- and the timetable refused, and the confirm dialog shows every
-- consequence of the edit at once rather than one function at a time.
-- Per-field setters were tried and removed;
-- they made the atomic unit smaller than the thing the user meant.
--
-- Editing follows the shared write principle:
-- apply the change, re-judge the enrolled students against stored
-- state, scoped to the rules whose inputs actually moved, and raise
-- YKV01 with the unaccepted set, the raise undoing everything.
-- Scoping by what moved is what keeps a name edit from resurfacing
-- an accepted placement.
--
-- Gates are not re-judged here or anywhere:
-- window and invite_only condition actions, not stored states,
-- so flipping invite_only never re-judges existing enrollments.
--
-- Array parameters are declarative and NULL reads as empty:
-- naming no periods means the course is unscheduled,
-- naming no grades means it is unrestricted on that axis.

-- The vocabulary a course write is about to reference, locked in the
-- global order: categories, then periods, then grades. Only ids that
-- name real rows are locked; a name that is not one is left to the
-- foreign key, which reports it as this element's rejection rather
-- than as a missing lock.
CREATE FUNCTION lock_vocabulary(
	p_category_id TEXT,
	p_period_ids TEXT[],
	p_grade_ids TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
	PERFORM lock_categories(COALESCE(
		(SELECT array_agg(c.id) FROM categories c WHERE c.id = p_category_id),
		'{}'));

	PERFORM lock_periods(COALESCE(
		(SELECT array_agg(DISTINCT p.id ORDER BY p.id)
		 FROM periods p WHERE p.id = ANY (COALESCE(p_period_ids, '{}'))),
		'{}'));

	PERFORM lock_grades(COALESCE(
		(SELECT array_agg(DISTINCT g.id ORDER BY g.id)
		 FROM grades g WHERE g.id = ANY (COALESCE(p_grade_ids, '{}'))),
		'{}'));
END;
$$;

-- Creating a course cannot violate anything:
-- it has no enrollees, so no rule has a subject.
-- It still takes locks, though, and that is why it is not the
-- one-statement LANGUAGE sql function it looks like it should be:
-- the inserts key-share the category, the periods and the grades they
-- name, and a write that took those after the course row would invert
-- the order a vocabulary deletion takes. See the lock order at the
-- head of 0013.
CREATE FUNCTION create_course(
	p_course_id entity_id,
	p_name trimmed_text,
	p_description TEXT,
	p_category_id entity_id,
	p_teacher trimmed_text_opt,
	p_teacher_email trimmed_text_opt,
	p_location trimmed_text_opt,
	p_term trimmed_text_opt,
	p_cost trimmed_text_opt,
	p_invite_only BOOLEAN,
	p_max_students count_value,
	p_period_ids TEXT[],
	p_legal_sexes legal_sex[],
	p_grade_ids TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
	PERFORM lock_vocabulary(p_category_id, p_period_ids, p_grade_ids);

	INSERT INTO courses
		(id, name, description, max_students, invite_only,
		 teacher, teacher_email, location, term, cost, category_id)
	VALUES (p_course_id, p_name, p_description, p_max_students,
		p_invite_only, p_teacher, p_teacher_email, p_location,
		p_term, p_cost, p_category_id);

	INSERT INTO course_periods (course_id, period_id)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_period_ids, '{}')) AS u;

	INSERT INTO course_allowed_legal_sexes (course_id, legal_sex)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_legal_sexes, '{}')) AS u;

	INSERT INTO course_allowed_grades (course_id, grade_id)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_grade_ids, '{}')) AS u;
END;
$$;

-- Editing a course.
--
-- The rules whose inputs this can move, and what moving them means:
--   periods       -> clash and budget, per enrolled student
--   legal sexes   -> legal_sex, per enrolled student
--   grades        -> grade, per enrolled student
--   max_students  -> overfull, once, for the course
-- Being over capacity is deliberately not judged per enrollee:
-- it is a fact about the course, so shrinking below the current
-- enrollment is one violation to accept, not twenty-seven identical
-- ones. Its code names the course alone.
--
-- It is a separate rule from "capacity", which is the admissions
-- question — whether one more student fits into a course that is
-- already full — and carries a code naming both the student and the
-- course. Two facts, two names: an administrator who accepted one
-- must not find that they have silently accepted the other.
--
-- Everything else — name, description, teacher, location, term,
-- cost, category, invite_only — is no rule's input and re-judges
-- nobody.
CREATE FUNCTION update_course(
	p_course_id entity_id,
	p_name trimmed_text,
	p_description TEXT,
	p_category_id entity_id,
	p_teacher trimmed_text_opt,
	p_teacher_email trimmed_text_opt,
	p_location trimmed_text_opt,
	p_term trimmed_text_opt,
	p_cost trimmed_text_opt,
	p_invite_only BOOLEAN,
	p_max_students count_value,
	p_period_ids TEXT[],
	p_legal_sexes legal_sex[],
	p_grade_ids TEXT[],
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_old_periods TEXT[];
	v_old_legal_sexes legal_sex[];
	v_old_grades TEXT[];
	v_old_max_students count_value;
	v_rules TEXT[] := '{}'::TEXT[];
	v_count BIGINT;
	v_unaccepted JSONB;
BEGIN
	-- The global order: the vocabulary this edit will reference,
	-- then the course, then its enrollees. Only the incoming sets
	-- need locking — deleting a referencing row locks nothing on the
	-- referenced side, so what the edit drops needs nothing.
	PERFORM lock_vocabulary(p_category_id, p_period_ids, p_grade_ids);

	PERFORM lock_courses(ARRAY[p_course_id]);

	PERFORM lock_students(COALESCE(
		(SELECT array_agg(e.student_id ORDER BY e.student_id)
		 FROM enrollments e
		 WHERE e.course_id = p_course_id),
		'{}'));

	-- The old sets, normalised the same way as the new ones,
	-- so that comparison sees a set rather than a spelling.
	SELECT c.max_students INTO v_old_max_students
	FROM courses c WHERE c.id = p_course_id;
	SELECT COALESCE(array_agg(DISTINCT cp.period_id::TEXT
		ORDER BY cp.period_id::TEXT), '{}'::TEXT[])
	INTO v_old_periods
	FROM course_periods cp WHERE cp.course_id = p_course_id;
	SELECT COALESCE(array_agg(DISTINCT a.legal_sex
		ORDER BY a.legal_sex), '{}'::legal_sex[])
	INTO v_old_legal_sexes
	FROM course_allowed_legal_sexes a WHERE a.course_id = p_course_id;
	SELECT COALESCE(array_agg(DISTINCT a.grade_id::TEXT
		ORDER BY a.grade_id::TEXT), '{}'::TEXT[])
	INTO v_old_grades
	FROM course_allowed_grades a WHERE a.course_id = p_course_id;

	UPDATE courses SET
		name = p_name,
		description = p_description,
		category_id = p_category_id,
		teacher = p_teacher,
		teacher_email = p_teacher_email,
		location = p_location,
		term = p_term,
		cost = p_cost,
		invite_only = p_invite_only,
		max_students = p_max_students
	WHERE id = p_course_id;

	DELETE FROM course_periods WHERE course_id = p_course_id;
	INSERT INTO course_periods (course_id, period_id)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_period_ids, '{}')) AS u;

	DELETE FROM course_allowed_legal_sexes WHERE course_id = p_course_id;
	INSERT INTO course_allowed_legal_sexes (course_id, legal_sex)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_legal_sexes, '{}')) AS u;

	DELETE FROM course_allowed_grades WHERE course_id = p_course_id;
	INSERT INTO course_allowed_grades (course_id, grade_id)
	SELECT DISTINCT p_course_id, u
	FROM unnest(COALESCE(p_grade_ids, '{}')) AS u;

	-- Which rules the edit actually moved.
	-- A widening is judged too, and comes back empty on its own:
	-- the protocol proves the change was harmless rather than
	-- assuming it from the direction.
	IF v_old_periods IS DISTINCT FROM (
		SELECT COALESCE(array_agg(DISTINCT u ORDER BY u), '{}'::TEXT[])
		FROM unnest(COALESCE(p_period_ids, '{}')) AS u)
	THEN
		v_rules := v_rules || ARRAY['clash', 'budget'];
	END IF;

	IF v_old_legal_sexes IS DISTINCT FROM (
		SELECT COALESCE(array_agg(DISTINCT u ORDER BY u), '{}'::legal_sex[])
		FROM unnest(COALESCE(p_legal_sexes, '{}')) AS u)
	THEN
		v_rules := v_rules || ARRAY['legal_sex'];
	END IF;

	IF v_old_grades IS DISTINCT FROM (
		SELECT COALESCE(array_agg(DISTINCT u ORDER BY u), '{}'::TEXT[])
		FROM unnest(COALESCE(p_grade_ids, '{}')) AS u)
	THEN
		v_rules := v_rules || ARRAY['grade'];
	END IF;

	-- Guarded, as in upsert_courses: with no rules to judge the
	-- result is empty either way, but the lateral still evaluates
	-- every rule for every enrollee before filtering, so a name-only
	-- edit of a large course would judge it in full for nothing.
	IF array_length(v_rules, 1) IS NOT NULL THEN
		v_unaccepted := course_enrollee_violations(
			p_course_id, v_rules, p_accept);
	ELSE
		v_unaccepted := '[]'::JSONB;
	END IF;

	IF p_max_students IS DISTINCT FROM v_old_max_students THEN
		SELECT count(*) INTO v_count
		FROM enrollments e WHERE e.course_id = p_course_id;
		IF v_count > p_max_students
			AND NOT ('overfull:' || p_course_id
				= ANY (COALESCE(p_accept, '{}')))
		THEN
			v_unaccepted := v_unaccepted || jsonb_build_object(
				'student_id', NULL,
				'rule', 'overfull',
				'code', 'overfull:' || p_course_id,
				'other_course_id', NULL,
				'period_id', NULL,
				'detail', format('%s is over its new capacity (%s/%s)',
					p_course_id, v_count, p_max_students));
		END IF;
	END IF;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;

-- Deleting a course removes its enrollments with it:
-- deletion is an explicit administrative act on the whole course,
-- and leaving the enrollments to block on the foreign key
-- would just force a remove_enrollments call first.
-- Periods and restrictions cascade by schema.
CREATE FUNCTION delete_course(p_course_id entity_id)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
	PERFORM lock_courses(ARRAY[p_course_id]);
	DELETE FROM enrollments WHERE course_id = p_course_id;
	DELETE FROM courses WHERE id = p_course_id;
END;
$$;

-- The course spreadsheet, as one call.
--
-- The department's master list is imported at the start of a season
-- and re-imported whenever it changes, so this is an upsert for the
-- same reason upsert_students is: absence from a file is not evidence
-- that a course was cancelled, and re-importing an unchanged file must
-- change nothing.
--
-- The parameter is JSONB rather than parallel arrays because a course
-- is ragged: each names its own period set and its own two restriction
-- sets, and no array shape carries that without inventing a delimiter.
-- That makes two places ragged input is genuine, the other being
-- set_grade_requirements; flat batches (students, enrollments) stay
-- typed arrays.
--
-- Each element runs in its own exception scope, so one malformed
-- course does not poison the file: domain, enum, foreign key and
-- cast rejections are collected into YKD01, with the element's index,
-- its id, the constraint it broke and the spreadsheet column it was
-- reading, so the administrator fixes the spreadsheet once. The
-- constraint name and the column are the machine-readable half: the
-- application turns them into prose a person can act on, where
-- SQLERRM names domains and relations and names no column at all. Malformed elements are not
-- violations — nothing about them is acceptable — and YKD01 is raised
-- before any violation reporting.
--
-- Re-judging is per element and scoped to the rules whose inputs that
-- element moved, exactly as update_course does it for one course:
--   periods       -> clash and budget, per enrolled student
--   legal sexes   -> legal_sex, per enrolled student
--   grades        -> grade, per enrolled student
--   max_students  -> overfull, once, for the course
-- A new course has no enrollees, so it re-judges nobody; an unchanged
-- re-imported row moves no rule's input and likewise re-judges
-- nobody, so a routine re-import never resurfaces an accepted
-- placement.
--
-- One subtransaction per element, as in upsert_students. A season is
-- a few hundred courses, well under the depth at which the
-- subtransaction cache becomes a concern.
CREATE FUNCTION upsert_courses(
	p_courses JSONB,
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_courses JSONB := COALESCE(p_courses, '[]'::JSONB);
	v_bad JSONB := '[]'::JSONB;
	-- What to judge, and against which rules, once every element has
	-- been applied. See the second pass below for why it cannot be
	-- done as we go. Rules are fixed identifiers with no commas in
	-- them, so one text per course carries the set.
	v_judge_ids TEXT[] := '{}'::TEXT[];
	v_judge_rules TEXT[] := '{}'::TEXT[];
	-- Counted separately from what v_bad holds, because the list is
	-- capped at max_reported_elements() and the count is not.
	v_bad_count INTEGER := 0;
	v_unaccepted JSONB := '[]'::JSONB;
	e JSONB;
	v_id TEXT;
	v_ok BOOLEAN;
	v_existed BOOLEAN;
	v_rules TEXT[];
	v_count BIGINT;
	v_new_max count_value;
	v_old_periods TEXT[];
	v_old_legal_sexes legal_sex[];
	v_old_grades TEXT[];
	v_old_max_students count_value;
	v_new_periods TEXT[];
	v_new_legal_sexes legal_sex[];
	v_new_grades TEXT[];
	v_element RECORD;
	i INTEGER;
	v_constraint TEXT;
	v_column TEXT;
	-- Which spreadsheet column the statement about to run is reading.
	-- A domain rejection carries the domain and the constraint but
	-- never a column: PostgreSQL raises it while casting a value,
	-- before the value has reached a column at all. So "must not be
	-- empty" arrived naming nothing, and a file whose term column was
	-- blank throughout came back as one identical anonymous sentence
	-- per row. The names here are the import's header names, because
	-- the person reading the message is looking at that header.
	v_field TEXT;
	v_new_id entity_id;
	v_new_name trimmed_text;
	v_new_teacher trimmed_text_opt;
	v_new_teacher_email trimmed_text_opt;
	v_new_location trimmed_text_opt;
	v_new_term trimmed_text_opt;
	v_new_cost trimmed_text_opt;
	v_new_category entity_id;
	v_new_invite BOOLEAN;
BEGIN
	IF jsonb_typeof(v_courses) IS DISTINCT FROM 'array' THEN
		RAISE EXCEPTION 'courses must be a JSON array'
			USING ERRCODE = '22023';
	END IF;

	-- The global lock order, taken up front over the whole batch:
	-- categories, periods, grades, courses, students, each ascending.
	-- Taking them per element would take them in the file's order
	-- instead, which is the client's, and two imports of overlapping
	-- lists would deadlock.
	--
	-- The vocabulary first, over the whole batch: every category,
	-- period and grade any element names. See the lock order at the
	-- head of 0013 for why these come before the courses.
	PERFORM lock_categories(COALESCE(
		(SELECT array_agg(DISTINCT c.id ORDER BY c.id)
		 FROM categories c
		 WHERE c.id IN (SELECT u->>'category_id'
			FROM jsonb_array_elements(v_courses) u)),
		'{}'));

	PERFORM lock_periods(COALESCE(
		(SELECT array_agg(DISTINCT p.id ORDER BY p.id)
		 FROM periods p
		 WHERE p.id IN (SELECT jsonb_array_elements_text(u->'period_ids')
			FROM jsonb_array_elements(v_courses) u
			WHERE jsonb_typeof(u->'period_ids') = 'array')),
		'{}'));

	PERFORM lock_grades(COALESCE(
		(SELECT array_agg(DISTINCT g.id ORDER BY g.id)
		 FROM grades g
		 WHERE g.id IN (SELECT jsonb_array_elements_text(u->'grade_ids')
			FROM jsonb_array_elements(v_courses) u
			WHERE jsonb_typeof(u->'grade_ids') = 'array')),
		'{}'));

	-- Only ids that name real rows are locked; a course the file
	-- creates has no row to lock yet, which is why the walk below is
	-- ordered as well: the insert is the first lock on a new row, so
	-- the order elements are visited in is itself a lock order.
	PERFORM lock_courses(COALESCE(
		(SELECT array_agg(DISTINCT c.id ORDER BY c.id)
		 FROM courses c
		 WHERE c.id IN (SELECT u->>'id'
			FROM jsonb_array_elements(v_courses) u)),
		'{}'));

	-- Every enrollee of every course in the batch: they are what the
	-- re-judging below reads, and their clash and budget facts must
	-- not move underneath it.
	PERFORM lock_students(COALESCE(
		(SELECT array_agg(DISTINCT en.student_id ORDER BY en.student_id)
		 FROM enrollments en
		 WHERE en.course_id IN (SELECT u->>'id'
			FROM jsonb_array_elements(v_courses) u)),
		'{}'));

	-- By id, not by the file's order. Courses are independent of one
	-- another — no course competes with another for anything — so the
	-- order is free to be imposed, and imposing one is what stops two
	-- administrators importing the same list sorted differently from
	-- holding each other's next row. The file's index travels
	-- alongside, because that is what a malformed element must be
	-- reported by.
	FOR v_element IN
		SELECT t.ord::INTEGER AS idx, t.value AS body
		FROM jsonb_array_elements(v_courses)
			WITH ORDINALITY AS t(value, ord)
		ORDER BY t.value->>'id', t.ord
	LOOP
		i := v_element.idx;
		e := v_element.body;
		v_id := e->>'id';

		-- The old sets, normalised the same way as the new ones, so
		-- that comparison sees a set rather than a spelling.
		SELECT c.max_students INTO v_old_max_students
		FROM courses c WHERE c.id = v_id;
		v_existed := FOUND;

		SELECT COALESCE(array_agg(DISTINCT cp.period_id::TEXT
			ORDER BY cp.period_id::TEXT), '{}'::TEXT[])
		INTO v_old_periods
		FROM course_periods cp WHERE cp.course_id = v_id;

		SELECT COALESCE(array_agg(DISTINCT als.legal_sex
			ORDER BY als.legal_sex), '{}'::legal_sex[])
		INTO v_old_legal_sexes
		FROM course_allowed_legal_sexes als WHERE als.course_id = v_id;

		SELECT COALESCE(array_agg(DISTINCT cag.grade_id::TEXT
			ORDER BY cag.grade_id::TEXT), '{}'::TEXT[])
		INTO v_old_grades
		FROM course_allowed_grades cag WHERE cag.course_id = v_id;

		v_ok := TRUE;

		BEGIN
			-- Every cast is inside this scope, so an ill-formed id,
			-- an unknown category, a non-numeric capacity, an
			-- unreadable boolean and a legal sex that is not one all
			-- arrive here as this element's rejection rather than the
			-- call's. Nothing above this needs to pre-validate, which
			-- is what lets a whole spreadsheet be judged in one pass.
			--
			-- Each cast is its own statement, preceded by the column
			-- it reads, rather than all of them inlined in the INSERT
			-- below. That is the whole difference between "this must
			-- not be empty" and "term must not be empty": the
			-- statement boundary is what carries the attribution,
			-- because the rejection itself does not.
			v_field := 'periods';
			SELECT COALESCE(array_agg(DISTINCT u ORDER BY u), '{}'::TEXT[])
			INTO v_new_periods
			FROM jsonb_array_elements_text(e->'period_ids') AS u;

			v_field := 'allowed_legal_sexes';
			SELECT COALESCE(array_agg(DISTINCT u::legal_sex
				ORDER BY u::legal_sex), '{}'::legal_sex[])
			INTO v_new_legal_sexes
			FROM jsonb_array_elements_text(e->'legal_sexes') AS u;

			v_field := 'allowed_grades';
			SELECT COALESCE(array_agg(DISTINCT u ORDER BY u), '{}'::TEXT[])
			INTO v_new_grades
			FROM jsonb_array_elements_text(e->'grade_ids') AS u;

			-- A blank cell and the word "unlimited" both mean no
			-- cap, which is NULL. A spreadsheet spells the absence
			-- of a limit both ways -- most rows leave the column
			-- empty, and the one that means it says so -- and
			-- neither spelling is a number, so both used to be a
			-- cast failure on every row that had no cap to state.
			-- Trimmed and folded first, because a spreadsheet's
			-- "Unlimited " is the same answer.
			v_field := 'max_students';
			v_new_max := NULLIF(NULLIF(
				lower(btrim(COALESCE(e->>'max_students', ''))),
				''), 'unlimited')::count_value;

			-- NULLIF before the cast, so an unfilled column means
			-- "not invite-only" rather than a cast failure: a
			-- spreadsheet leaves cells empty.
			v_field := 'invite_only';
			v_new_invite :=
				COALESCE(NULLIF(e->>'invite_only', '')::BOOLEAN, FALSE);

			v_field := 'id';
			v_new_id := (e->>'id')::entity_id;

			v_field := 'name';
			v_new_name := (e->>'name')::trimmed_text;

			v_field := 'teacher';
			v_new_teacher := COALESCE(e->>'teacher', '')::trimmed_text_opt;

			v_field := 'teacher_email';
			v_new_teacher_email :=
				COALESCE(e->>'teacher_email', '')::trimmed_text_opt;

			v_field := 'location';
			v_new_location := COALESCE(e->>'location', '')::trimmed_text_opt;

			v_field := 'term';
			v_new_term := COALESCE(e->>'term', '')::trimmed_text_opt;

			v_field := 'cost';
			v_new_cost := COALESCE(e->>'cost', '')::trimmed_text_opt;

			v_field := 'category';
			v_new_category := (e->>'category_id')::entity_id;

			-- Everything is cast by now, so the only rejections left
			-- here are the category foreign key and a NULL where a
			-- column is NOT NULL -- and the latter names its own
			-- column, which is why COLUMN_NAME is read below.
			INSERT INTO courses (id, name, description, max_students,
				invite_only, teacher, teacher_email, location,
				term, cost, category_id)
			VALUES (
				v_new_id,
				v_new_name,
				COALESCE(e->>'description', ''),
				v_new_max,
				v_new_invite,
				v_new_teacher,
				v_new_teacher_email,
				v_new_location,
				v_new_term,
				v_new_cost,
				v_new_category)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				max_students = EXCLUDED.max_students,
				invite_only = EXCLUDED.invite_only,
				teacher = EXCLUDED.teacher,
				teacher_email = EXCLUDED.teacher_email,
				location = EXCLUDED.location,
				term = EXCLUDED.term,
				cost = EXCLUDED.cost,
				category_id = EXCLUDED.category_id;

			-- The three sets are replaced wholesale, as in
			-- update_course: the file states the arrangement it wants.
			-- Each carries its own column name, because an unknown
			-- period and an unknown grade are the same foreign key
			-- violation to the reader of a message that names neither.
			v_field := 'periods';
			DELETE FROM course_periods WHERE course_id = v_id;
			INSERT INTO course_periods (course_id, period_id)
			SELECT v_id, u FROM unnest(v_new_periods) AS u;

			v_field := 'allowed_legal_sexes';
			DELETE FROM course_allowed_legal_sexes WHERE course_id = v_id;
			INSERT INTO course_allowed_legal_sexes (course_id, legal_sex)
			SELECT v_id, u FROM unnest(v_new_legal_sexes) AS u;

			v_field := 'allowed_grades';
			DELETE FROM course_allowed_grades WHERE course_id = v_id;
			INSERT INTO course_allowed_grades (course_id, grade_id)
			SELECT v_id, u FROM unnest(v_new_grades) AS u;
		EXCEPTION WHEN check_violation OR not_null_violation
			OR foreign_key_violation OR invalid_text_representation
			OR invalid_parameter_value OR numeric_value_out_of_range
		THEN
			GET STACKED DIAGNOSTICS v_constraint = CONSTRAINT_NAME,
				v_column = COLUMN_NAME;
			v_bad_count := v_bad_count + 1;

			IF v_bad_count <= max_reported_elements() THEN
				v_bad := v_bad || jsonb_build_object(
					'index', i,
					'id', COALESCE(v_id, ''),
					'sqlstate', SQLSTATE,
					'constraint', COALESCE(v_constraint, ''),
					'field', COALESCE(v_field, ''),
					'column', COALESCE(v_column, ''),
					'message', SQLERRM);
			END IF;

			v_ok := FALSE;
		END;

		CONTINUE WHEN NOT v_ok;

		-- Which rules this element actually moved. A widening is
		-- judged too, and comes back empty on its own: the protocol
		-- proves the change was harmless rather than assuming it from
		-- the direction.
		v_rules := '{}'::TEXT[];

		IF v_old_periods IS DISTINCT FROM v_new_periods THEN
			v_rules := v_rules || ARRAY['clash', 'budget'];
		END IF;

		IF v_old_legal_sexes IS DISTINCT FROM v_new_legal_sexes THEN
			v_rules := v_rules || ARRAY['legal_sex'];
		END IF;

		IF v_old_grades IS DISTINCT FROM v_new_grades THEN
			v_rules := v_rules || ARRAY['grade'];
		END IF;

		IF array_length(v_rules, 1) IS NOT NULL THEN
			v_judge_ids := v_judge_ids || v_id;
			v_judge_rules := v_judge_rules
				|| array_to_string(v_rules, ',');
		END IF;

		-- Capacity is a fact about the course, so it is reported once
		-- for the course rather than once per enrollee, and its code
		-- names the course alone.
		IF v_existed AND v_new_max IS DISTINCT FROM v_old_max_students THEN
			SELECT count(*) INTO v_count
			FROM enrollments en WHERE en.course_id = v_id;

			IF v_count > v_new_max
				AND NOT ('overfull:' || v_id
					= ANY (COALESCE(p_accept, '{}')))
			THEN
				v_unaccepted := v_unaccepted || jsonb_build_object(
					'student_id', NULL,
					'rule', 'overfull',
					'code', 'overfull:' || v_id,
					'other_course_id', NULL,
					'period_id', NULL,
					'detail', format('%s is over its new capacity (%s/%s)',
						v_id, v_count, v_new_max));
			END IF;
		END IF;
	END LOOP;

	IF v_bad_count > 0 THEN
		RAISE EXCEPTION '% malformed element(s)', v_bad_count
			USING ERRCODE = 'YKD01', DETAIL = v_bad::TEXT;
	END IF;

	-- Judge only now, with the whole batch applied.
	--
	-- Judging inside the loop judged a state this call was still in
	-- the middle of building: when the first element was judged, every
	-- later element still held its old periods. clash and budget are
	-- the two rules that read another course's periods, so both
	-- reported facts about a half-applied catalogue. The ordinary case
	-- is two courses trading timetable slots — a re-import where AAA
	-- moves to P2 and BBB moves to P1 — which was refused with
	-- "clashes with BBB in P2" for a clash that does not exist before
	-- the import and does not exist after it. The only way past was to
	-- accept a violation that was never true, which is exactly what
	-- the accept protocol is for not having to do.
	--
	-- This is the charter in 0012 read properly: inside the
	-- transaction the stored state is the hypothesis, and the
	-- hypothesis is not finished until the last element has landed.
	FOR j IN 1 .. COALESCE(array_length(v_judge_ids, 1), 0) LOOP
		v_unaccepted := v_unaccepted || course_enrollee_violations(
			v_judge_ids[j],
			string_to_array(v_judge_rules[j], ','),
			p_accept);
	END LOOP;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;
