-- Enrollment read models.
--
-- Read models embed the display names they reference, so a response is
-- self-sufficient: a client showing an enrollment does not have to
-- hold the student and course tables to put a name on it.
--
-- The cost of that is a hand-maintained invalidation graph, and it is
-- paid rather than avoided: renaming a student changes what v_enrollments
-- says, and thirty-five Broadcast(WSMessage("invalidate_...")) calls
-- across internal/web are what say so. Whoever adds a view or a
-- handler adds an edge, and nothing checks that they did.
--
-- This comment used to claim the events layer derived those edges from
-- the database's own dependency tracking. It does not, and never did:
-- pg_depend appears nowhere in this repository, there are no triggers
-- and no LISTEN/NOTIFY. Deriving them was a proposal in
-- docs/review-2026-08-10.md and is still one. A maintainer who trusted
-- the sentence would add a view and ship a permanently stale display.

CREATE VIEW v_enrollments AS
SELECT
	e.student_id,
	s.name AS student_name,
	s.grade_id,
	e.course_id,
	c.name AS course_name,
	e.student_droppable,
	e.counts_toward_budget
FROM enrollments e
JOIN students s ON s.id = e.student_id
JOIN courses c ON c.id = e.course_id;

-- Per-student derived numbers, one row per student.
--
-- budgeted_periods_used is the total periods of the student's
-- counts_toward_budget enrollments;
-- a course meeting twice a week counts 2,
-- and a course with no periods yet charges 0.
--
-- distinct_categories_used counts distinct categories
-- over the student's enrollments in scheduled courses only
-- (courses with at least one period):
-- an unscheduled course occupies no time
-- and therefore satisfies nothing.
--
-- Both are advisory reads;
-- enforcement of the budget belongs to the write layer.
CREATE VIEW v_student_status AS
SELECT
	s.id AS student_id,
	s.name AS student_name,
	s.grade_id,
	(SELECT count(*)
	 FROM enrollments e
	 JOIN course_periods cp ON cp.course_id = e.course_id
	 WHERE e.student_id = s.id
		AND e.counts_toward_budget)::BIGINT AS budgeted_periods_used,
	g.max_budgeted_periods,
	(SELECT count(DISTINCT c.category_id)
	 FROM enrollments e
	 JOIN courses c ON c.id = e.course_id
	 WHERE e.student_id = s.id
		AND EXISTS (SELECT 1
			FROM course_periods cp
			WHERE cp.course_id = c.id))::BIGINT AS distinct_categories_used,
	g.min_distinct_categories
FROM students s
JOIN grades g ON g.id = s.grade_id;

-- Requirement satisfaction,
-- one row per student x requirement group of their grade.
--
-- satisfied_periods counts the periods of the student's enrollments
-- in courses whose category is in the requirement's member set,
-- whatever the enrollment's policy bits:
-- occupancy is occupancy,
-- so an invited placement satisfies requirements
-- exactly like an own pick.
-- Unscheduled courses contribute no periods.
--
-- Advisory: reported to students and administrators,
-- never enforced as a transition rule.
CREATE VIEW v_student_requirements AS
SELECT
	s.id AS student_id,
	s.grade_id,
	rg.id AS requirement_category_id,
	rg.min_period_count,
	sat.satisfied_periods,
	sat.satisfied_periods >= rg.min_period_count AS met
FROM students s
JOIN grade_requirement_groups rg ON rg.grade_id = s.grade_id
CROSS JOIN LATERAL (
	SELECT count(*)::BIGINT AS satisfied_periods
	FROM enrollments e
	JOIN courses c ON c.id = e.course_id
	JOIN course_periods cp ON cp.course_id = e.course_id
	WHERE e.student_id = s.id
		AND c.category_id IN (SELECT m.category_id
			FROM grade_requirement_group_categories m
			WHERE m.requirement_category_id = rg.id)
) sat;

-- The PowerSchool hand-off: one row per (student, course, period)
-- occupancy cell, flat and denormalised because the receiving end
-- is a spreadsheet.
--
-- The join to course_periods is outer:
-- a course still being scheduled meets in no period,
-- and hiding its enrollees would under-report the roster.
-- Such a row carries a NULL period_id.
--
-- period_sort_order is exposed only so a reader can order by it:
-- an ORDER BY inside a view is not carried through an outer query,
-- so the ordering has to be stated where the rows are selected.
CREATE VIEW v_export_enrollments AS
SELECT
	s.id AS student_id,
	s.name AS student_name,
	s.grade_id,
	s.legal_sex,
	c.id AS course_id,
	c.name AS course_name,
	c.term,
	cp.period_id,
	p.sort_order AS period_sort_order,
	e.student_droppable,
	e.counts_toward_budget
FROM enrollments e
JOIN students s ON s.id = e.student_id
JOIN courses c ON c.id = e.course_id
LEFT JOIN course_periods cp ON cp.course_id = c.id
LEFT JOIN periods p ON p.id = cp.period_id;
