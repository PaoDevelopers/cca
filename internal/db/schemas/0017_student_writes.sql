-- Student writes.
--
-- upsert_students is the import and the single edit alike:
-- insert new students, update existing ones, never delete
-- (absence from a file is not evidence of leaving).
-- Deleting students is plain query-layer DML,
-- blocked by the enrollments foreign key while enrolled.
-- Grades must pre-exist; an unknown grade is a hard error.
--
-- Parameters arrive as TEXT arrays and are cast per element,
-- so one malformed element does not poison the whole call:
-- each element runs in its own exception scope,
-- and domain, enum, and foreign key rejections are collected into
--   YKD01  malformed elements.
--          DETAIL is a JSON array of
--          {index, id, sqlstate, constraint, field, column, message},
--          every bad element, so the admin fixes the file once.
--          The constraint name and the column are carried because
--          they are the machine-readable half: the application turns
--          them into prose a person can act on, where SQLERRM names
--          domains and constraints and names no column at all.
-- Malformed elements are not violations:
-- nothing about them is acceptable, so no accept applies,
-- and YKD01 is raised before any violation reporting.
--
-- Rule re-judging follows the shared principle,
-- scoped to the fields an element actually changed
-- (IS DISTINCT FROM the old row):
-- a changed grade_id re-judges that student's enrollments
-- for 'grade' (per enrollment) and 'budget' (once, per student);
-- a changed legal_sex re-judges for 'legal_sex';
-- a changed name, or an unchanged re-imported row,
-- re-judges nothing,
-- so routine re-imports never resurface accepted placements.
CREATE FUNCTION upsert_students(
	p_ids TEXT[],
	p_names TEXT[],
	p_grade_ids TEXT[],
	p_legal_sexes TEXT[],
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_n INTEGER := COALESCE(array_length(p_ids, 1), 0);
	v_bad JSONB := '[]'::JSONB;
	-- Counted separately from what v_bad holds, because the list
	-- is capped at max_reported_elements() and the count is not.
	v_bad_count INTEGER := 0;
	v_unaccepted JSONB := '[]'::JSONB;
	v_old RECORD;
	v_existed BOOLEAN;
	v_element RECORD;
	v_caps_differ BOOLEAN;
	i INTEGER;
	v_constraint TEXT;
	v_column TEXT;
	-- Which spreadsheet column the statement about to run is reading.
	-- A domain rejection carries the domain and the constraint but
	-- never a column: it is raised while casting a value, before the
	-- value has reached a column at all. Without this, a roster whose
	-- name column had a stray leading space came back as the same
	-- anonymous "must not be empty" as a blank one. The names are the
	-- import's header names, because that is what the person reading
	-- the message is looking at.
	v_field TEXT;
	v_new_id localpart;
	v_new_name trimmed_text;
	v_new_grade entity_id;
	v_new_legal_sex legal_sex;
BEGIN
	-- NULL is not the empty roster.
	--
	-- A Go nil slice arrives here as a NULL array, and every length
	-- below then agrees at zero, so the batch runs, walks nothing, and
	-- reports success. An import that silently loaded no students is
	-- worse than one that failed: an administrator watches it succeed
	-- and moves on.
	--
	-- The empty roster is spelt as an empty array, which is a
	-- different value and is still accepted as the no-op it is.
	IF p_ids IS NULL OR p_names IS NULL
		OR p_grade_ids IS NULL OR p_legal_sexes IS NULL
	THEN
		RAISE EXCEPTION 'upsert_students: the parameter arrays must not be NULL'
			USING ERRCODE = '22023';
	END IF;

	IF array_length(p_names, 1) IS DISTINCT FROM NULLIF(v_n, 0)
		OR array_length(p_grade_ids, 1) IS DISTINCT FROM NULLIF(v_n, 0)
		OR array_length(p_legal_sexes, 1) IS DISTINCT FROM NULLIF(v_n, 0)
	THEN
		RAISE EXCEPTION 'parameter arrays must have equal lengths'
			USING ERRCODE = '22023';
	END IF;

	-- Grades first, then students: the global lock order.
	-- The grade locks are not optional bookkeeping —
	-- writing a student's grade_id takes a FOR KEY SHARE lock on the
	-- referenced grade as part of the foreign key check, so without
	-- taking them up front this function would run
	-- students-then-grades and deadlock against
	-- set_max_budgeted_periods. Malformed grade ids are left to the
	-- per-element handling below, so only real ones are locked here.
	PERFORM lock_grades(COALESCE(
		(SELECT array_agg(DISTINCT g.id ORDER BY g.id)
		 FROM grades g
		 WHERE g.id = ANY (p_grade_ids)),
		'{}'));

	-- Existing rows among the ids, locked ascending;
	-- a student the file creates has no row to lock yet, which is why
	-- the walk below is ordered as well: the insert is the first lock
	-- on a new row, so the order elements are visited in is itself a
	-- lock order. Two administrators loading the same roster sorted
	-- differently would otherwise hold each other's next row.
	--
	-- Today the grade lock above happens to serialize whole batches
	-- that share a grade, which hides this. That is an accident of
	-- rosters being grouped by grade, not a property to rely on.
	PERFORM 1 FROM students s
	WHERE s.id = ANY (p_ids)
	ORDER BY s.id
	FOR UPDATE;

	-- By id, not by the file's order. Students are independent of one
	-- another, so the order is free to be imposed; the file's index
	-- travels alongside, because that is what a malformed element is
	-- reported by.
	FOR v_element IN
		SELECT u.ord::INTEGER AS idx
		FROM unnest(p_ids) WITH ORDINALITY AS u(id, ord)
		ORDER BY u.id, u.ord
	LOOP
		i := v_element.idx;
		SELECT s.grade_id, s.legal_sex INTO v_old
		FROM students s WHERE s.id = p_ids[i];
		v_existed := FOUND;

		BEGIN
			-- One statement per column, rather than four casts
			-- inlined in the INSERT. The statement boundary is what
			-- carries the attribution: the rejection itself names a
			-- domain and a constraint, never a column.
			v_field := 'id';
			v_new_id := p_ids[i]::localpart;

			v_field := 'name';
			v_new_name := p_names[i]::trimmed_text;

			v_field := 'grade';
			v_new_grade := p_grade_ids[i]::entity_id;

			v_field := 'legal_sex';
			v_new_legal_sex := p_legal_sexes[i]::legal_sex;

			-- Only the grade foreign key and the NOT NULLs are left,
			-- and a NOT NULL names its own column, which is why
			-- COLUMN_NAME is read below.
			v_field := 'grade';
			INSERT INTO students (id, name, grade_id, legal_sex)
			VALUES (v_new_id, v_new_name, v_new_grade, v_new_legal_sex)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name,
				grade_id = EXCLUDED.grade_id,
				legal_sex = EXCLUDED.legal_sex;
		EXCEPTION WHEN check_violation OR not_null_violation
			OR foreign_key_violation OR invalid_text_representation
		THEN
			GET STACKED DIAGNOSTICS v_constraint = CONSTRAINT_NAME,
				v_column = COLUMN_NAME;
			v_bad_count := v_bad_count + 1;

			IF v_bad_count <= max_reported_elements() THEN
				v_bad := v_bad || jsonb_build_object(
					'index', i,
					'id', p_ids[i],
					'sqlstate', SQLSTATE,
					'constraint', COALESCE(v_constraint, ''),
					'field', COALESCE(v_field, ''),
					'column', COALESCE(v_column, ''),
					'message', SQLERRM);
			END IF;

			CONTINUE;
		END;

		-- A new grade changes which restrictions this student meets
		-- and which cap their budget is measured against.
		IF v_existed
			AND v_old.grade_id IS DISTINCT FROM p_grade_ids[i]
		THEN
			v_unaccepted := v_unaccepted
				|| student_enrollment_violations(
					p_ids[i], ARRAY['grade'], p_accept);

			-- The budget rule counts periods against the grade's cap,
			-- and moving between two grades that cap the same number
			-- cannot change the comparison: the student occupies what
			-- they occupied and the bound is what it was. Judging it
			-- anyway re-ran the rule against a student whose
			-- enrollments an administrator had already accepted over
			-- the cap, and refused — so a roster re-import that merely
			-- corrected somebody's year group was blocked by a
			-- decision already taken. Same shape as the fix to
			-- set_enrollment_policy: judge the transition, not the
			-- statement.
			SELECT (SELECT g.max_budgeted_periods FROM grades g
					WHERE g.id = v_old.grade_id)
				IS DISTINCT FROM
				(SELECT g.max_budgeted_periods FROM grades g
					WHERE g.id = p_grade_ids[i])
			INTO v_caps_differ;

			IF v_caps_differ THEN
				v_unaccepted := v_unaccepted
					|| student_budget_violation(p_ids[i], p_accept);
			END IF;
		END IF;

		IF v_existed
			AND v_old.legal_sex::TEXT IS DISTINCT FROM p_legal_sexes[i]
		THEN
			v_unaccepted := v_unaccepted
				|| student_enrollment_violations(
					p_ids[i], ARRAY['legal_sex'], p_accept);
		END IF;
	END LOOP;

	IF v_bad_count > 0 THEN
		RAISE EXCEPTION '% malformed element(s)', v_bad_count
			USING ERRCODE = 'YKD01', DETAIL = v_bad::TEXT;
	END IF;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;
