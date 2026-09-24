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
-- The rules are written once, in course_violations below, over a set
-- of courses; enrollment_violations is that set with one member, and
-- student_course_violations is it with every course the student does
-- not hold.
--
-- STABLE and lock-free:
-- advisory when the UI or the confirm dialog calls it,
-- authoritative when write functions re-evaluate it
-- inside their locks.
-- Both named rows are assumed to exist;
-- existence is the write functions' concern.
-- The candidates are p_course_ids, or, when p_every_unheld is set,
-- every course the student does not hold (p_course_ids is then
-- ignored). A flag rather than the list itself because the list would
-- have to be a subquery in the caller's arguments, and PostgreSQL does
-- not inline a function called with one: the body would be planned
-- afresh on every call, which cost more than the rules themselves.
CREATE FUNCTION course_violations(
	p_student_id localpart,
	p_course_ids TEXT[],
	p_every_unheld BOOLEAN,
	p_counts_toward_budget BOOLEAN,
	p_disregard_course_ids TEXT[]
)
RETURNS TABLE (
	course_id TEXT,
	rule TEXT,
	code TEXT,
	other_course_id TEXT,
	period_id TEXT,
	detail TEXT
)
LANGUAGE sql
STABLE
BEGIN ATOMIC
-- Why over a set, when the question is about one course.
--
-- The catalogue asks it about every course at once, and asked one
-- course at a time — a lateral over a single-course function — every
-- fact about the student was looked up again for each course: their
-- row, their grade, their enrollments and the periods those occupy,
-- and the budget they have used, fifty times over for fifty courses.
-- That read was the most expensive in the system and it is asked the
-- most often. Over a set, the student's facts are read once and each
-- rule is one join against the candidate courses; the rules are the
-- same, and so is every row they produce.
WITH
	cand (c_id) AS (
		SELECT u
		FROM unnest(p_course_ids) AS u
		WHERE NOT p_every_unheld
		UNION
		SELECT c.id
		FROM courses c
		WHERE p_every_unheld
			AND NOT EXISTS (SELECT 1
				FROM enrollments e
				WHERE e.student_id = p_student_id
					AND e.course_id = c.id)
	),
	st (s_id, s_grade_id, s_legal_sex, s_max_budgeted_periods) AS (
		SELECT s.id, s.grade_id, s.legal_sex, g.max_budgeted_periods
		FROM students s
		JOIN grades g ON g.id = s.grade_id
		WHERE s.id = p_student_id
	),
	-- The student's enrollments, less those being dropped in the same
	-- operation (a swap).
	mine (m_course_id, m_counts_toward_budget) AS (
		SELECT e.course_id, e.counts_toward_budget
		FROM enrollments e
		WHERE e.student_id = p_student_id
			AND NOT (e.course_id = ANY (COALESCE(p_disregard_course_ids, '{}')))
	),
	-- Periods per candidate course: what enrolling would occupy.
	cand_periods (cp_course_id, cp_n) AS (
		SELECT cand.c_id, count(cp.period_id)
		FROM cand
		LEFT JOIN course_periods cp ON cp.course_id = cand.c_id
		GROUP BY cand.c_id
	),
	-- Budgeted periods the student already occupies, all courses.
	-- The budget rule below takes a candidate's own enrollment back
	-- out, so that re-judging an existing enrollment does not count its
	-- periods both as used and as new.
	used (u_n) AS (
		SELECT count(*)
		FROM mine m
		JOIN course_periods cp ON cp.course_id = m.m_course_id
		WHERE m.m_counts_toward_budget
	)
SELECT *
FROM (
	SELECT cand.c_id AS v_course_id,
		'legal_sex'::TEXT AS v_rule,
		'legal_sex:' || p_student_id || ':' || cand.c_id AS v_code,
		NULL::TEXT AS v_other_course_id,
		NULL::TEXT AS v_period_id,
		format('legal sex %s is not allowed', st.s_legal_sex) AS v_detail
	FROM cand
	CROSS JOIN st
	WHERE EXISTS (SELECT 1
			FROM course_allowed_legal_sexes a
			WHERE a.course_id = cand.c_id)
		AND NOT EXISTS (SELECT 1
			FROM course_allowed_legal_sexes a
			WHERE a.course_id = cand.c_id
				AND a.legal_sex = st.s_legal_sex)

	UNION ALL

	SELECT cand.c_id,
		'grade',
		'grade:' || p_student_id || ':' || cand.c_id,
		NULL, NULL,
		format('grade %s is not allowed', st.s_grade_id)
	FROM cand
	CROSS JOIN st
	WHERE EXISTS (SELECT 1
			FROM course_allowed_grades a
			WHERE a.course_id = cand.c_id)
		AND NOT EXISTS (SELECT 1
			FROM course_allowed_grades a
			WHERE a.course_id = cand.c_id
				AND a.grade_id = st.s_grade_id)

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
	SELECT cand.c_id,
		'capacity',
		'capacity:' || p_student_id || ':' || cand.c_id,
		NULL, NULL,
		format('%s is full (%s/%s)',
			cand.c_id, v.current_students, v.max_students)
	FROM cand
	JOIN v_courses v ON v.id = cand.c_id
	WHERE v.current_students >= v.max_students

	UNION ALL

	-- One row per (candidate, other course, shared period).
	SELECT cand.c_id,
		'clash',
		'clash:' || p_student_id || ':'
			|| least(cand.c_id, m.m_course_id::TEXT) || ':'
			|| greatest(cand.c_id, m.m_course_id::TEXT) || ':'
			|| cpn.period_id,
		m.m_course_id,
		cpn.period_id,
		format('Clashes with %s (%s) in %s',
			other_course.name, m.m_course_id, cpn.period_id)
	FROM mine m
	JOIN courses other_course ON other_course.id = m.m_course_id
	JOIN course_periods cpo ON cpo.course_id = m.m_course_id
	JOIN course_periods cpn ON cpn.period_id = cpo.period_id
	JOIN cand ON cand.c_id = cpn.course_id
	WHERE m.m_course_id <> cand.c_id

	UNION ALL

	-- A NULL cap means no cap:
	-- the comparison is then NULL and produces no row.
	SELECT cand.c_id,
		'budget',
		'budget:' || p_student_id,
		NULL, NULL,
		format('would occupy %s of %s budgeted periods',
			b.n, st.s_max_budgeted_periods)
	FROM cand
	CROSS JOIN st
	JOIN cand_periods cpc ON cpc.cp_course_id = cand.c_id
	CROSS JOIN used
	CROSS JOIN LATERAL (
		SELECT used.u_n
			- CASE WHEN EXISTS (SELECT 1
					FROM mine m
					WHERE m.m_course_id = cand.c_id
						AND m.m_counts_toward_budget)
				THEN cpc.cp_n ELSE 0 END
			+ cpc.cp_n AS n
	) b
	WHERE p_counts_toward_budget
		AND b.n > st.s_max_budgeted_periods
) AS v (v_course_id, v_rule, v_code, v_other_course_id, v_period_id, v_detail);
END;

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
SELECT v.rule, v.code, v.other_course_id, v.period_id, v.detail
FROM course_violations(p_student_id, ARRAY[p_course_id::TEXT], FALSE,
	p_counts_toward_budget, p_disregard_course_ids) v;
END;

-- Every candidate course judged for one student, in one call.
--
-- The student catalogue needs the violation set of every course
-- at once, to say why a course cannot be taken
-- before the student tries.
-- It is course_violations over every course, so the rules keep their
-- single definition and the student's own facts are read once rather
-- than once per course.
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
	SELECT v.course_id, v.rule, v.code, v.other_course_id,
		v.period_id, v.detail
	FROM course_violations(p_student_id, '{}', TRUE,
		p_counts_toward_budget, '{}') v;
END;
