-- The one course read model, serving students and administrators
-- alike: no course attribute is secret,
-- and restrictions shown to students explain their ineligibility
-- better than a bare rejection would.
-- Strictly viewer-independent:
-- per-student facts (enrolled? eligible?) belong to
-- parameterized reads, never here.
--
-- current_students is where a course's enrollee count is published:
-- the realtime mirror and the browser both read it from here.
--
-- The capacity checks in 0015 count the enrollments table directly
-- rather than reading this, because they run inside a write that has
-- just changed it and must see their own effect. Same number, counted
-- in two places; if one ever changes, the other has to.
--
-- Association arrays are ordered by their entities' sort_order
-- inside the aggregate, so their order follows administrator
-- reordering; consumers must treat them as sets with a
-- presentation order, not as stable sequences.
-- An empty array means no periods scheduled
-- or no restriction on that axis.
CREATE VIEW v_courses AS
SELECT
	c.id,
	c.name,
	c.description,
	c.category_id,
	c.teacher,
	c.teacher_email,
	c.location,
	c.term,
	c.cost,
	c.max_students,
	c.invite_only,
	(SELECT count(*)
	 FROM enrollments e
	 WHERE e.course_id = c.id)::BIGINT AS current_students,
	COALESCE(
		(SELECT array_agg(cp.period_id ORDER BY p.sort_order, cp.period_id)
		 FROM course_periods cp
		 JOIN periods p ON p.id = cp.period_id
		 WHERE cp.course_id = c.id),
		'{}')::TEXT[] AS period_ids,
	COALESCE(
		(SELECT array_agg(als.legal_sex ORDER BY als.legal_sex)
		 FROM course_allowed_legal_sexes als
		 WHERE als.course_id = c.id),
		'{}')::legal_sex[] AS allowed_legal_sexes,
	COALESCE(
		(SELECT array_agg(cag.grade_id ORDER BY g.sort_order, cag.grade_id)
		 FROM course_allowed_grades cag
		 JOIN grades g ON g.id = cag.grade_id
		 WHERE cag.course_id = c.id),
		'{}')::TEXT[] AS allowed_grade_ids
FROM courses c;
