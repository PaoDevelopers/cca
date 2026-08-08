-- name: GetGrades :many
SELECT * FROM v_grades ORDER BY sort_order, id;

-- name: GetGradeRequirements :many
SELECT * FROM v_grade_requirements ORDER BY grade_id, id;

-- name: NewGrade :exec
INSERT INTO grades
	(id, name, opens_at, closes_at,
	 max_budgeted_periods, min_distinct_categories, sort_order)
SELECT sqlc.arg(id), sqlc.arg(name), NULL, NULL,
	sqlc.narg(max_budgeted_periods), sqlc.arg(min_distinct_categories),
	COALESCE(MAX(sort_order), 0) + 1
FROM grades;

-- name: DeleteGrade :execrows
DELETE FROM grades WHERE id = $1;

-- name: SetGradeOrder :exec
SELECT set_grade_order($1);

-- name: SetMaxBudgetedPeriods :exec
SELECT set_max_budgeted_periods(sqlc.arg(grade_id),
	sqlc.narg(max_budgeted_periods), sqlc.arg(accept));

-- Each bound is written only if the caller named it.
--
-- Not both-or-neither, because the two boxes on the card are edited
-- separately and by different people. An administrator setting the
-- closing date would otherwise send back whatever opening time their
-- page was built with, which is not what the grade has if somebody
-- else moved it since — and nobody would see it happen, because the
-- write succeeds and restores a value that had genuinely been there.
--
-- NULL is a value here ("no such bound"), so absence cannot be spelt
-- as NULL and needs a flag of its own.
-- name: SetGradeWindow :execrows
UPDATE grades SET
	opens_at = CASE WHEN sqlc.arg(set_opens_at)::BOOLEAN
		THEN sqlc.narg(opens_at)::TIMESTAMPTZ ELSE opens_at END,
	closes_at = CASE WHEN sqlc.arg(set_closes_at)::BOOLEAN
		THEN sqlc.narg(closes_at)::TIMESTAMPTZ ELSE closes_at END
WHERE id = sqlc.arg(id);

-- The two manual levers, as operations of the clock that decides.
--
-- The bounds above are instants an administrator chose, so they come
-- from the administrator. "Now" is not: it is a reading of a clock,
-- and the only clock whose reading matters is the one v_grades.is_open
-- is evaluated against. Taken in the browser it was a third clock —
-- after the database's and the Go process's — and it arrived rounded
-- to the minute, because that is all a datetime-local box carries. Two
-- presses inside one minute then wrote closes_at = opens_at and were
-- refused by the CHECK, leaving the window open and the administrator
-- holding an error about a value they never typed.
--
-- Opening leaves closes_at alone. A schedule is a thing an
-- administrator set on purpose, and starting a window early is not a
-- statement about when it should end; erasing the end would be one,
-- made silently. The cost is that a closing time already in the past
-- cannot be kept and opened around — the CHECK forbids the row — so
-- that case is refused rather than resolved by discarding it. Hence
-- two facts, not a row count: no such grade and cannot-open-around are
-- different answers and the caller has to tell them apart.
-- name: OpenGradeWindowNow :one
WITH target AS (
	-- Cast because the domain does not survive inference through the
	-- CTE: without it sqlc types the parameter pgtype.Text, and an
	-- id that is never NULL starts travelling as one that might be.
	SELECT id, closes_at FROM grades WHERE id = sqlc.arg(id)::entity_id
), updated AS (
	UPDATE grades g SET opens_at = now()
	FROM target t
	WHERE g.id = t.id
		AND (t.closes_at IS NULL OR t.closes_at > now())
	RETURNING g.id
)
SELECT EXISTS (SELECT 1 FROM target) AS found,
	EXISTS (SELECT 1 FROM updated) AS opened;

-- Closing writes closes_at and leaves opens_at alone: that is when the
-- window ran, and once it is gone the card can no longer say what
-- happened, only that nothing is happening.
--
-- Guarded on v_grades.is_open rather than on a bound comparison
-- written out again here. Openness has one definition and this is a
-- caller of it, not a second author of it — and getting it slightly
-- wrong is not hypothetical: guarding on opens_at < now() alone also
-- matches a window that opened and closed yesterday, so closing an
-- already-shut grade overwrote the record of when it really ran.
--
-- The guard also makes closes_at > opens_at hold by construction: a
-- window that is open began before now. Zero rows means there was
-- nothing to close, which is a true answer worth giving rather than an
-- error; a window merely scheduled to open later is cancelled by
-- clearing opens_at, not by closing it.
-- name: CloseGradeWindowNow :execrows
-- Cast for the same reason as the opener above: the domain does not
-- survive inference past the subquery, and an id that is never NULL
-- should not travel as one that might be.
UPDATE grades SET closes_at = now()
WHERE id = sqlc.arg(id)::entity_id
	AND EXISTS (SELECT 1 FROM v_grades v
		WHERE v.id = grades.id AND v.is_open);

-- The two fields of a grade that are no rule's input, written
-- together because they are one form and one save. Two statements
-- would be two transactions on the pool: the first could land, the
-- second fail, and the response would report failure for an edit that
-- half happened.
-- name: UpdateGradeSettings :execrows
UPDATE grades
SET name = $2, min_distinct_categories = $3
WHERE id = $1;

-- name: SetGradeRequirements :exec
SELECT set_grade_requirements($1, $2);

-- The next window boundary in the future, over every grade, so the
-- realtime layer can arm one timer at it. NULL when no grade has a
-- boundary still ahead.
-- name: NextWindowBoundary :one
SELECT min(t)::TIMESTAMPTZ AS next_boundary
FROM (
	SELECT opens_at AS t FROM grades
	UNION ALL
	SELECT closes_at FROM grades
) b
WHERE t > now();
