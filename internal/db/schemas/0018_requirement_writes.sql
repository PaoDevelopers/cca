-- Requirement-group writes.
--
-- A grade's requirements are one form, so they are set as a whole:
-- the caller names the arrangement it wants and the function makes
-- the stored state match. That makes the operation idempotent,
-- like every other declarative administrator write, and it removes
-- the read-modify-write the per-requirement shape forced on callers.
--
-- The parameter is JSONB rather than parallel arrays because
-- requirements are ragged — each names its own category set —
-- and no array shape carries that without inventing a delimiter.
-- This is the one place ragged input is genuine;
-- flat batches (students, enrollments) stay typed arrays.
--
-- Requirements are advisory: they are reported, never enforced as a
-- transition rule, so nothing here judges enrollments or accepts
-- violations. Replacing them cannot break anything, only change what
-- a student is told.
--
-- It still takes locks, because it writes rows that reference the
-- vocabulary and the doctrine binds every write regardless of what it
-- means: a referenced row is locked before the row that references
-- it, so categories come before grades. Inserting into
-- grade_requirement_group_categories key-shares its category whether
-- or not this function says so, and doing that after the grade lock
-- is the whole of the deadlock against lock_vocabulary.
--
-- The ids are internal and nothing durable references them,
-- so replacement is delete-and-reinsert and the ids churn.
--
-- A requirement over no categories is unsatisfiable when its minimum
-- is positive and inert otherwise — malformed input either way,
-- rejected as 22023 rather than surfaced as a violation.
CREATE FUNCTION set_grade_requirements(
	p_grade_id entity_id,
	p_requirements JSONB
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_id BIGINT;
	v_form JSONB := COALESCE(p_requirements, '[]'::JSONB);
	v_categories TEXT[];
	r JSONB;
BEGIN
	IF jsonb_typeof(v_form) IS DISTINCT FROM 'array' THEN
		RAISE EXCEPTION 'requirements must be a JSON array'
			USING ERRCODE = '22023';
	END IF;

	-- The whole form is checked before anything is locked or written.
	-- Validating inside the write loop would leave the earlier
	-- requirements' locks held while the transaction unwinds, and
	-- would make which rows got as far as being inserted depend on
	-- the order the caller happened to write them in.
	FOR r IN SELECT e FROM jsonb_array_elements(v_form) e
	LOOP
		IF jsonb_typeof(r->'category_ids') IS DISTINCT FROM 'array'
			OR jsonb_array_length(r->'category_ids') = 0
		THEN
			RAISE EXCEPTION
				'requirement must name at least one category'
				USING ERRCODE = '22023';
		END IF;

		-- Checked rather than cast blind: a missing key would reach
		-- the NOT NULL as a bare 23502 and a non-numeric one as a
		-- bare 22P02, neither of which is in the vocabulary this
		-- function documents.
		IF r->>'min_period_count' IS NULL
			OR r->>'min_period_count' !~ '^[0-9]+$'
		THEN
			RAISE EXCEPTION
				'requirement must name a whole number of periods'
				USING ERRCODE = '22023';
		END IF;
	END LOOP;

	-- Categories first, then the grade. lock_categories orders them
	-- among themselves and reports a category that does not exist as
	-- P0002, which is a better answer than the 23503 the insert would
	-- otherwise raise from inside the loop.
	SELECT array_agg(DISTINCT u) INTO v_categories
	FROM jsonb_array_elements(v_form) e,
		jsonb_array_elements_text(e->'category_ids') AS u;
	PERFORM lock_categories(COALESCE(v_categories, ARRAY[]::TEXT[]));

	-- Locked, not merely read. The replacement below is a delete
	-- followed by inserts, which under READ COMMITTED is not a
	-- replacement at all when two administrators save the same form
	-- at once: the second deletes rows the first already removed,
	-- inserts its own, and the grade ends up holding the union of two
	-- arrangements rather than the later one.
	PERFORM 1 FROM grades g WHERE g.id = p_grade_id FOR UPDATE;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'grade % not found', p_grade_id
			USING ERRCODE = 'P0002';
	END IF;

	DELETE FROM grade_requirement_groups WHERE grade_id = p_grade_id;

	FOR r IN SELECT e FROM jsonb_array_elements(v_form) e
	LOOP
		INSERT INTO grade_requirement_groups
			(grade_id, min_period_count)
		VALUES (p_grade_id, (r->>'min_period_count')::BIGINT)
		RETURNING id INTO v_id;

		INSERT INTO grade_requirement_group_categories
			(requirement_category_id, category_id)
		SELECT DISTINCT v_id, u
		FROM jsonb_array_elements_text(r->'category_ids') AS u;
	END LOOP;
END;
$$;
