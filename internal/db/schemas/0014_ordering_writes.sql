-- Display order for grades and periods.
--
-- Reordering is declarative: the caller names the whole order,
-- and the function assigns 1..N by position.
-- That is the shape of the operation as an administrator performs it
-- (a list is dragged into an arrangement and saved),
-- and it makes reordering idempotent —
-- the same call twice leaves the same order,
-- and a double-clicked button cannot shuffle anything.
--
-- The relative alternative (move this one up) was rejected:
-- it is not idempotent, it cannot express the arrangement the
-- administrator is looking at, and it forces sort_order to be
-- maintained gapless and unique, which in turn forces deferred
-- constraints and an exclusive table lock on every create and delete.
-- Declaring the order removes all of that:
-- creating a grade is an INSERT and deleting one is a DELETE, both
-- plain query-layer statements with no explicit locking. (The DELETE
-- still takes FOR KEY SHARE on every row that references the grade,
-- because that is how RESTRICT is proved; see the head of 0013.)
--
-- They do take locks, though — one per row, because they renumber
-- every row. Left to the plan, those are acquired in whatever order
-- the array happens to be in, which is the caller's; two
-- administrators dragging the same list into different arrangements
-- then hold each other's next row. So each locks its rows ascending
-- first, through the same lock vocabulary as every other write.
-- Two concurrent creations can pick the same sort_order,
-- and a creation concurrent with a reorder can land outside 1..N;
-- both are ties or gaps, both are broken deterministically by id
-- where the order is read, and the next reorder repairs them.
-- Nothing depends on the numbers themselves.

-- Assigns 1..N to the grades named, in the order named.
-- The list must name every grade exactly once:
-- a partial order is not an order, and silently leaving rows behind
-- would produce an arrangement the administrator did not ask for.
CREATE FUNCTION set_grade_order(p_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_updated BIGINT;
BEGIN
	IF (SELECT count(DISTINCT u) FROM unnest(p_ids) AS u)
		IS DISTINCT FROM COALESCE(array_length(p_ids, 1), 0)
	THEN
		RAISE EXCEPTION 'the order must not name a grade twice'
			USING ERRCODE = '22023';
	END IF;

	-- Ascending, before the renumbering takes them in the caller's
	-- order. A name that is not a grade is left to the count check
	-- below, so only real ids are locked here.
	PERFORM lock_grades(COALESCE(
		(SELECT array_agg(DISTINCT g.id ORDER BY g.id)
		 FROM grades g WHERE g.id = ANY (p_ids)),
		'{}'));

	UPDATE grades g SET sort_order = o.ord
	FROM unnest(p_ids) WITH ORDINALITY AS o (id, ord)
	WHERE g.id = o.id;
	GET DIAGNOSTICS v_updated = ROW_COUNT;

	-- Catches both a name that is not a grade and a grade that was
	-- not named: either way the count of rows moved is wrong.
	IF v_updated <> (SELECT count(*) FROM grades) THEN
		RAISE EXCEPTION 'the order must name every grade exactly once'
			USING ERRCODE = '22023';
	END IF;
END;
$$;

CREATE FUNCTION set_period_order(p_ids TEXT[])
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
	v_updated BIGINT;
BEGIN
	IF (SELECT count(DISTINCT u) FROM unnest(p_ids) AS u)
		IS DISTINCT FROM COALESCE(array_length(p_ids, 1), 0)
	THEN
		RAISE EXCEPTION 'the order must not name a period twice'
			USING ERRCODE = '22023';
	END IF;

	PERFORM lock_periods(COALESCE(
		(SELECT array_agg(DISTINCT p.id ORDER BY p.id)
		 FROM periods p WHERE p.id = ANY (p_ids)),
		'{}'));

	UPDATE periods p SET sort_order = o.ord
	FROM unnest(p_ids) WITH ORDINALITY AS o (id, ord)
	WHERE p.id = o.id;
	GET DIAGNOSTICS v_updated = ROW_COUNT;

	IF v_updated <> (SELECT count(*) FROM periods) THEN
		RAISE EXCEPTION 'the order must name every period exactly once'
			USING ERRCODE = '22023';
	END IF;
END;
$$;
