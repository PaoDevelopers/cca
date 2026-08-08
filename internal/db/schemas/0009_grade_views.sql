-- The single definition of window openness.
--
-- Its consumers are the three self_* write functions, whose gate this
-- is, and the grades read that both frontends display. They read
-- is_open from here rather than restating the expression, so what a
-- student is shown and what they are allowed cannot drift.
--
-- Not every view and not the violation checks: 0012 excludes the
-- actor-keyed gates deliberately, because on the administrator path
-- window openness is not a violation at all.
CREATE VIEW v_grades AS
SELECT
	id,
	name,
	opens_at,
	closes_at,
	(opens_at IS NOT NULL AND opens_at <= now()
		AND (closes_at IS NULL OR now() < closes_at)) AS is_open,
	max_budgeted_periods,
	min_distinct_categories,
	sort_order
FROM grades;

-- One row per requirement, members aggregated.
-- Member ids are ordered inside the aggregate
-- because that is the one ordering an outer query cannot impose;
-- display names are the consumer's to map from its categories data.
CREATE VIEW v_grade_requirements AS
SELECT
	rg.id,
	rg.grade_id,
	rg.min_period_count,
	COALESCE(
		(SELECT array_agg(m.category_id ORDER BY m.category_id)
		 FROM grade_requirement_group_categories m
		 WHERE m.requirement_category_id = rg.id),
		'{}')::TEXT[] AS category_ids
FROM grade_requirement_groups rg;
