-- The acceptable-violation set for one prospective enrollment:
-- "if p_student_id were enrolled in p_course_id now,
-- which acceptable rules would that break?"
--
-- The negotiable rules that can be asked about a course a student does
-- not hold live here: legal_sex, grade, capacity, clash, and budget.
--
-- One negotiable rule is not here: `overfull`, in 0015, which asks
-- whether lowering a course's capacity would strand students already
-- enrolled. It is negotiable in the same sense — p_accept takes it —
-- but it is a question about a write to a course rather than about a
-- student and a course, so it has nowhere to live in this shape.
-- The actor-keyed gates (window openness, invite-only membership)
-- are deliberately absent:
-- on the admin path they are not violations at all,
-- and on the student path they are absolute,
-- checked by the write functions against the same read models
-- the UI displays.
--
-- p_counts_toward_budget is the enrollment's own bit;
-- the budget rule fires only when it is set.
-- p_disregard_course_ids lists courses being dropped
-- in the same operation (a swap):
-- they are excluded from clash and budget computations,
-- as if already gone.
-- Array parameters treat NULL as empty:
-- Go nil slices arrive as NULL,
-- and x = ANY (NULL) would otherwise silently void the predicate.
--
-- Always judges stored state.
-- Writes that change a rule's inputs apply the change first,
-- re-judge the affected enrollments through this function,
-- and raise unless accepted (the raise undoing everything),
-- so no counterfactual parameters are needed:
-- inside the transaction, the stored state is the hypothesis.
--
-- A violation's identity is its code:
-- the violated fact named absolutely,
-- the same string whichever write surfaces it:
--   rule ':' student ':' course, plus for 'clash'
--   ':' the other course ':' period;
--   'budget' carries the student alone,
--   being a per-student aggregate.
-- Codes are injective because ':' is excluded
-- from every component grammar.
--
-- A clash is the one rule whose subject is a pair rather than a
-- course, so its two course components are written in sorted order,
-- not in the order the judging happened to take them. Without that,
-- the same physical clash has two spellings — one from each side —
-- and an administrator who accepts it while placing a student is
-- asked to accept it again the moment the other course is edited,
-- because the re-judge surfaces it from the opposite axis. The
-- other_course_id column still reports the other course as seen from
-- the subject, which is what a reader wants; only the identity is
-- canonical.
--
-- STABLE and lock-free:
-- advisory when the UI or the confirm dialog calls it,
-- authoritative when write functions re-evaluate it
-- inside their locks.
-- Both named rows are assumed to exist;
-- existence is the write functions' concern.
CREATE FUNCTION enrollment_violations(
	p_student_id localpart,
	p_course_id entity_id,
	p_counts_toward_budget BOOLEAN,
	p_disregard_course_ids TEXT[]
)
RETURNS TABLE (
	rule TEXT,
	code TEXT,
	other_course_id TEXT,
	period_id TEXT,
	detail TEXT
)
LANGUAGE sql
STABLE
BEGIN ATOMIC
SELECT *
FROM (
	SELECT 'legal_sex'::TEXT AS v_rule,
		'legal_sex:' || p_student_id || ':' || p_course_id AS v_code,
		NULL::TEXT AS v_other_course_id,
		NULL::TEXT AS v_period_id,
		format('legal sex %s is not allowed', s.legal_sex) AS v_detail
	FROM students s
	WHERE s.id = p_student_id
		AND EXISTS (SELECT 1
			FROM course_allowed_legal_sexes a
			WHERE a.course_id = p_course_id)
		AND NOT EXISTS (SELECT 1
			FROM course_allowed_legal_sexes a
			WHERE a.course_id = p_course_id
				AND a.legal_sex = s.legal_sex)

	UNION ALL

	SELECT 'grade',
		'grade:' || p_student_id || ':' || p_course_id,
		NULL, NULL,
		format('grade %s is not allowed', s.grade_id)
	FROM students s
	WHERE s.id = p_student_id
		AND EXISTS (SELECT 1
			FROM course_allowed_grades a
			WHERE a.course_id = p_course_id)
		AND NOT EXISTS (SELECT 1
			FROM course_allowed_grades a
			WHERE a.course_id = p_course_id
				AND a.grade_id = s.grade_id)

	UNION ALL

	-- The admissions question: whether one more student fits. Named
	-- for both the student and the course, and distinct from the
	-- "overfull" rule the course writes raise, which is the different
	-- question of whether a course now holds more than its cap.
	--
	-- current_students comes from v_courses,
	-- the single definition of the count.
	--
	-- An uncapped course has max_students NULL, and the comparison
	-- below is then NULL rather than true, so it yields no row: no
	-- test for the absence is needed or wanted. The format() above is
	-- never reached for such a course.
	SELECT 'capacity',
		'capacity:' || p_student_id || ':' || p_course_id,
		NULL, NULL,
		format('%s is full (%s/%s)',
			p_course_id, v.current_students, v.max_students)
	FROM v_courses v
	WHERE v.id = p_course_id
		AND v.current_students >= v.max_students

	UNION ALL

	-- One row per (other course, shared period).
	SELECT 'clash',
		'clash:' || p_student_id || ':'
			|| least(p_course_id::TEXT, e.course_id::TEXT) || ':'
			|| greatest(p_course_id::TEXT, e.course_id::TEXT) || ':'
			|| cpn.period_id,
		e.course_id,
		cpn.period_id,
		format('Clashes with %s (%s) in %s',
			other_course.name, e.course_id, cpn.period_id)
	FROM enrollments e
	JOIN courses other_course ON other_course.id = e.course_id
	JOIN course_periods cpo ON cpo.course_id = e.course_id
	JOIN course_periods cpn ON cpn.period_id = cpo.period_id
		AND cpn.course_id = p_course_id
	WHERE e.student_id = p_student_id
		AND e.course_id <> p_course_id
		AND NOT (e.course_id = ANY (COALESCE(p_disregard_course_ids, '{}')))

	UNION ALL

	-- A NULL cap means no cap:
	-- the comparison is then NULL and produces no row.
	SELECT 'budget',
		'budget:' || p_student_id,
		NULL, NULL,
		format('would occupy %s of %s budgeted periods',
			used.n + new.n, g.max_budgeted_periods)
	FROM students s
	JOIN grades g ON g.id = s.grade_id
	CROSS JOIN LATERAL (
		-- The student's enrollment in p_course_id itself is
		-- excluded: re-judging an existing enrollment must not
		-- count the course's periods both as used and as new.
		SELECT count(*) AS n
		FROM enrollments e
		JOIN course_periods cp ON cp.course_id = e.course_id
		WHERE e.student_id = p_student_id
			AND e.counts_toward_budget
			AND e.course_id <> p_course_id
			AND NOT (e.course_id = ANY (COALESCE(p_disregard_course_ids, '{}')))
	) used
	CROSS JOIN LATERAL (
		SELECT count(*) AS n
		FROM course_periods cp
		WHERE cp.course_id = p_course_id
	) new
	WHERE s.id = p_student_id
		AND p_counts_toward_budget
		AND used.n + new.n > g.max_budgeted_periods
) AS v (v_rule, v_code, v_other_course_id, v_period_id, v_detail);
END;

-- Every candidate course judged for one student, in one call.
--
-- The student catalogue needs the violation set of every course
-- at once, to say why a course cannot be taken
-- before the student tries.
-- Asking enrollment_violations once per course
-- is one round trip per course;
-- this is a lateral over the same function,
-- so the rules keep their single definition
-- and PostgreSQL inlines the body into the join.
--
-- Courses the student already holds are excluded:
-- eligibility is a question about courses they might enter,
-- and a held course would otherwise report its own capacity
-- against the student occupying one of its seats.
--
-- p_counts_toward_budget is the bit the prospective enrollment
-- would carry; the student path always passes TRUE,
-- an administrator previewing an unbudgeted placement passes FALSE.
CREATE FUNCTION student_course_violations(
	p_student_id localpart,
	p_counts_toward_budget BOOLEAN
)
RETURNS TABLE (
	course_id TEXT,
	rule TEXT,
	code TEXT,
	other_course_id TEXT,
	period_id TEXT,
	detail TEXT
)
STABLE
BEGIN ATOMIC
	SELECT c.id, v.rule, v.code, v.other_course_id,
		v.period_id, v.detail
	FROM courses c
	CROSS JOIN LATERAL enrollment_violations(
		p_student_id, c.id, p_counts_toward_budget, '{}') v
	WHERE NOT EXISTS (
		SELECT 1 FROM enrollments e
		WHERE e.student_id = p_student_id AND e.course_id = c.id);
END;
