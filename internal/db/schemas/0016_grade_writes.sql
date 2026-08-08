-- Grade parameter writes.
--
-- Only max_budgeted_periods is a violation-rule input;
-- changing it follows the shared write principle:
-- apply, re-judge the grade's students scoped to budget,
-- raise YKV01 with the unaccepted set.
-- The window bounds are gate inputs,
-- and min_distinct_categories and the name are advisory or cosmetic:
-- all plain query-layer DML, re-judging nobody.
--
-- Locks: the grade row, then the grade's students ascending, which is
-- the global order — grades come before students, and no course row is
-- taken here at all. (Grades come before courses too; a restatement
-- here once had it the other way round, which is the direction that
-- deadlocks against upsert_courses.)
-- Concurrent placements lock the target student,
-- so cap changes and placements serialize per student.
CREATE FUNCTION set_max_budgeted_periods(
	p_grade_id entity_id,
	p_max_budgeted_periods count_value,
	p_accept TEXT[]
)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_unaccepted JSONB;
	v_old count_value;
BEGIN
	PERFORM lock_grades(ARRAY[p_grade_id]);

	-- Did the input actually move?
	--
	-- This is a declarative write: the caller states the cap it wants
	-- and the function makes the stored state match, so saving the
	-- same form twice must be a no-op. Without this guard it is not.
	-- The second save re-judges every student in the grade against a
	-- cap nobody changed, and any student who was already over it —
	-- because an administrator deliberately accepted that earlier, or
	-- because a forced placement put them there — comes back as an
	-- unaccepted violation. The administrator is then asked to
	-- re-accept a decision they already made, in order to complete a
	-- write that would have changed nothing.
	--
	-- Read under the grade lock taken above, so no concurrent write
	-- can move it between the read and the update. A grade that does
	-- not exist was already refused by lock_grades, so a NULL here
	-- means the cap is unset, not that the row is missing.
	SELECT g.max_budgeted_periods INTO v_old
	FROM grades g WHERE g.id = p_grade_id;

	IF p_max_budgeted_periods IS NOT DISTINCT FROM v_old THEN
		RETURN;
	END IF;

	PERFORM lock_students(COALESCE(
		(SELECT array_agg(s.id ORDER BY s.id)
		 FROM students s
		 WHERE s.grade_id = p_grade_id),
		'{}'));

	UPDATE grades SET max_budgeted_periods = p_max_budgeted_periods
	WHERE id = p_grade_id;

	SELECT COALESCE(jsonb_agg(a.u ORDER BY s.id, a.ord), '[]'::JSONB)
	INTO v_unaccepted
	FROM students s
	CROSS JOIN LATERAL jsonb_array_elements(
		student_budget_violation(s.id, p_accept))
		WITH ORDINALITY AS a (u, ord)
	WHERE s.grade_id = p_grade_id;

	PERFORM raise_unaccepted(v_unaccepted);
END;
$$;
