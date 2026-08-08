-- A period is an indivisible timeslot.
-- The system never computes with wall-clock time or day structure,
-- only period identity;
-- the name may carry wall-clock as inert prose
-- ('Monday 16:00-17:00').
-- Periods are disjoint by definition.
CREATE TABLE periods (
	id entity_id PRIMARY KEY,
	name trimmed_text NOT NULL UNIQUE,

	-- Display order, as in grades.
	sort_order count_value NOT NULL CHECK (sort_order > 0)
);
