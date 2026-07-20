-- Development-only fixture data for exercising both React application areas.
-- Run with: psql -v ON_ERROR_STOP=1 -d cca -f dev/mock_data.sql
--
-- The script is additive and safe to run repeatedly. Existing sessions are
-- preserved, while fixture metadata is refreshed to the values below.

BEGIN;

INSERT INTO grades (grade, enabled, max_own_choices)
VALUES
	('10', TRUE, 4),
	('11', TRUE, 4),
	('12', TRUE, 4)
ON CONFLICT (grade) DO UPDATE
SET enabled = EXCLUDED.enabled,
	max_own_choices = EXCLUDED.max_own_choices;

INSERT INTO categories (id)
VALUES
	('STEM'),
	('Arts'),
	('Sports'),
	('Community'),
	('Leadership')
ON CONFLICT (id) DO NOTHING;

-- The 16 Monday-Thursday CCA slots are immutable system data seeded by the
-- schema. This fixture only assigns courses to those existing slots.

INSERT INTO courses (
	id,
	name,
	description,
	max_students,
	membership,
	teacher,
	location,
	category_id
)
VALUES
	('BASKET', 'Basketball Team', 'Team training with a choice of Monday or Tuesday CCA 1.', 20, 'free', 'Ms Wilson', 'Sports Hall', 'Sports'),
	('CODE', 'Competitive Coding', 'Algorithms, contest practice, and collaborative problem solving.', 18, 'free', 'Mr Patel', 'Computer Lab 2', 'STEM'),
	('ROBO', 'Robotics Club', 'Build and program robots for team challenges.', 20, 'free', 'Mr Smith', 'Room 101', 'STEM'),
	('MAKER', 'Maker Lab', 'Prototype practical projects with electronics and digital fabrication.', 16, 'free', 'Ms Chan', 'Innovation Lab', 'STEM'),
	('SCIRES', 'Advanced Science Research', 'Student-led research projects for Grades 11 and 12.', 12, 'free', 'Dr Evans', 'Science Lab 4', 'STEM'),
	('PAINT', 'Visual Arts Studio', 'Painting, printmaking, and portfolio development.', 16, 'free', 'Mr Lee', 'Art Studio', 'Arts'),
	('DRAMA', 'Drama Production', 'Rehearsal and production work for the school play.', 24, 'free', 'Ms Brooks', 'Black Box Theatre', 'Arts'),
	('ORCH', 'School Orchestra', 'Audition-based ensemble rehearsing a varied concert programme.', 30, 'invite_only', 'Mr Wong', 'Music Hall', 'Arts'),
	('YEARBOOK', 'Yearbook Editorial Team', 'Photography, writing, layout, and publication planning.', 14, 'free', 'Ms Green', 'Media Room', 'Arts'),
	('DANCE', 'Dance Company', 'Technique, choreography, and performance rehearsals.', 20, 'free', 'Ms Garcia', 'Dance Studio', 'Arts'),
	('FOOTBALL', 'Football Club', 'Skills training and friendly match preparation.', 24, 'free', 'Mr Brown', 'Main Field', 'Sports'),
	('BADMINTON', 'Badminton Club', 'Technique drills, doubles tactics, and match play.', 20, 'free', 'Ms Lin', 'Sports Hall', 'Sports'),
	('CHESS', 'Chess Club', 'Tactical training and a small internal league.', 3, 'free', 'Mr Adams', 'Library Seminar Room', 'Leadership'),
	('SERVICE', 'Community Service', 'Plan and deliver weekly service projects with local partners.', 30, 'free', 'Ms Taylor', 'Community Hub', 'Community'),
	('MUN', 'Model United Nations', 'Research, public speaking, negotiation, and conference preparation.', 18, 'free', 'Mr Clark', 'Humanities Room 3', 'Leadership'),
	('PHOTO', 'Photography Basics', 'Camera fundamentals and visual storytelling projects.', 2, 'free', 'Ms Young', 'Media Room', 'Arts'),
	('GVOL', 'Girls Volleyball', 'Team training and match preparation.', 18, 'free', 'Ms Roberts', 'Sports Hall', 'Sports')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
	description = EXCLUDED.description,
	max_students = EXCLUDED.max_students,
	membership = EXCLUDED.membership,
	teacher = EXCLUDED.teacher,
	location = EXCLUDED.location,
	category_id = EXCLUDED.category_id;

WITH desired (course_id, period_id) AS (
	VALUES
		('BASKET', 'Monday CCA 1'),
		('BASKET', 'Tuesday CCA 1'),
		('CODE', 'Monday CCA 2'),
		('CODE', 'Wednesday CCA 2'),
		('ROBO', 'Monday CCA 3'),
		('MAKER', 'Thursday CCA 1'),
		('SCIRES', 'Tuesday CCA 1'),
		('SCIRES', 'Thursday CCA 2'),
		('PAINT', 'Wednesday CCA 1'),
		('DRAMA', 'Tuesday CCA 2'),
		('DRAMA', 'Thursday CCA 2'),
		('ORCH', 'Monday CCA 2'),
		('ORCH', 'Thursday CCA 1'),
		('YEARBOOK', 'Thursday CCA 2'),
		('DANCE', 'Wednesday CCA 2'),
		('DANCE', 'Thursday CCA 3'),
		('FOOTBALL', 'Wednesday CCA 1'),
		('FOOTBALL', 'Thursday CCA 3'),
		('BADMINTON', 'Tuesday CCA 2'),
		('CHESS', 'Monday CCA 1'),
		('SERVICE', 'Thursday CCA 4'),
		('MUN', 'Monday CCA 1'),
		('MUN', 'Thursday CCA 2'),
		('PHOTO', 'Wednesday CCA 1'),
		('GVOL', 'Tuesday CCA 1'),
		('GVOL', 'Thursday CCA 1')
)
INSERT INTO course_periods (course_id, period_id)
SELECT d.course_id, d.period_id
FROM desired d
WHERE NOT EXISTS (
	SELECT 1
	FROM course_periods cp
	WHERE cp.course_id = d.course_id
		AND cp.period_id = d.period_id
);

-- Demonstrate grade and legal-sex eligibility filters.
WITH desired (course_id, grade) AS (
	VALUES
		('SCIRES', '11'),
		('SCIRES', '12')
)
INSERT INTO course_allowed_grades (course_id, grade)
SELECT d.course_id, d.grade
FROM desired d
WHERE NOT EXISTS (
	SELECT 1
	FROM course_allowed_grades cag
	WHERE cag.course_id = d.course_id
		AND cag.grade = d.grade
);

INSERT INTO course_allowed_legal_sexes (course_id, legal_sex)
SELECT 'GVOL', 'F'::legal_sex
WHERE NOT EXISTS (
	SELECT 1
	FROM course_allowed_legal_sexes cals
	WHERE cals.course_id = 'GVOL'
		AND cals.legal_sex = 'F'
);

INSERT INTO students (id, name, grade, legal_sex)
VALUES
	(23321, 'Test Student', '10', 'M'),
	(22537, 'Henry Yang', '10', 'M'),
	(33445, 'Test Student', '11', 'F'),
	(10001, 'Ava Chen', '10', 'F'),
	(10002, 'Leo Wang', '10', 'M'),
	(10003, 'Alex Morgan', '10', 'X'),
	(10004, 'Mia Li', '11', 'F'),
	(10005, 'Ethan Zhang', '11', 'M'),
	(10006, 'Jordan Taylor', '11', 'X'),
	(10007, 'Sophia Liu', '12', 'F'),
	(10008, 'Noah Wu', '12', 'M'),
	(10009, 'Riley Chen', '12', 'X'),
	(10010, 'Grace Sun', '10', 'F'),
	(10011, 'Daniel Zhao', '11', 'M'),
	(10012, 'Sam Lee', '12', 'X')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
	grade = EXCLUDED.grade,
	legal_sex = EXCLUDED.legal_sex;

-- Two requirement groups per grade: one broad academic/creative group and one
-- sport/service group. Stable high IDs keep repeat loads deterministic.
INSERT INTO grade_requirement_groups (id, grade, min_count)
VALUES
	(900001, '10', 1),
	(900002, '10', 1),
	(900003, '11', 1),
	(900004, '11', 1),
	(900005, '12', 1),
	(900006, '12', 1)
ON CONFLICT (id) DO UPDATE
SET grade = EXCLUDED.grade,
	min_count = EXCLUDED.min_count;

WITH desired (req_group_id, category_id) AS (
	VALUES
		(900001::BIGINT, 'STEM'),
		(900001::BIGINT, 'Arts'),
		(900001::BIGINT, 'Leadership'),
		(900002::BIGINT, 'Sports'),
		(900002::BIGINT, 'Community'),
		(900003::BIGINT, 'STEM'),
		(900003::BIGINT, 'Arts'),
		(900003::BIGINT, 'Leadership'),
		(900004::BIGINT, 'Sports'),
		(900004::BIGINT, 'Community'),
		(900005::BIGINT, 'STEM'),
		(900005::BIGINT, 'Arts'),
		(900005::BIGINT, 'Leadership'),
		(900006::BIGINT, 'Sports'),
		(900006::BIGINT, 'Community')
)
INSERT INTO grade_requirement_group_categories (req_group_id, category_id)
SELECT d.req_group_id, d.category_id
FROM desired d
WHERE NOT EXISTS (
	SELECT 1
	FROM grade_requirement_group_categories grgc
	WHERE grgc.req_group_id = d.req_group_id
		AND grgc.category_id = d.category_id
);

-- A mixture of normal, invited, full-capacity, and multi-slot selections.
WITH desired (student_id, course_id, selection_type) AS (
	VALUES
		(22537::BIGINT, 'BASKET', 'normal'::selection_type),
		(22537::BIGINT, 'CODE', 'normal'::selection_type),
		(22537::BIGINT, 'SERVICE', 'normal'::selection_type),
		(33445::BIGINT, 'ORCH', 'invite'::selection_type),
		(33445::BIGINT, 'BADMINTON', 'normal'::selection_type),
		(33445::BIGINT, 'SERVICE', 'normal'::selection_type),
		(10001::BIGINT, 'BASKET', 'normal'::selection_type),
		(10001::BIGINT, 'PAINT', 'normal'::selection_type),
		(10001::BIGINT, 'DANCE', 'normal'::selection_type),
		(10002::BIGINT, 'FOOTBALL', 'normal'::selection_type),
		(10002::BIGINT, 'CODE', 'normal'::selection_type),
		(10002::BIGINT, 'CHESS', 'normal'::selection_type),
		(10003::BIGINT, 'DRAMA', 'normal'::selection_type),
		(10003::BIGINT, 'MAKER', 'normal'::selection_type),
		(10003::BIGINT, 'SERVICE', 'normal'::selection_type),
		(10003::BIGINT, 'PHOTO', 'normal'::selection_type),
		(10004::BIGINT, 'BASKET', 'normal'::selection_type),
		(10004::BIGINT, 'PAINT', 'normal'::selection_type),
		(10004::BIGINT, 'YEARBOOK', 'normal'::selection_type),
		(10005::BIGINT, 'CODE', 'normal'::selection_type),
		(10005::BIGINT, 'FOOTBALL', 'normal'::selection_type),
		(10005::BIGINT, 'MAKER', 'normal'::selection_type),
		(10006::BIGINT, 'DANCE', 'normal'::selection_type),
		(10006::BIGINT, 'BADMINTON', 'normal'::selection_type),
		(10006::BIGINT, 'CHESS', 'normal'::selection_type),
		(10007::BIGINT, 'ORCH', 'invite'::selection_type),
		(10007::BIGINT, 'FOOTBALL', 'normal'::selection_type),
		(10007::BIGINT, 'YEARBOOK', 'normal'::selection_type),
		(10008::BIGINT, 'BASKET', 'normal'::selection_type),
		(10008::BIGINT, 'DRAMA', 'normal'::selection_type),
		(10008::BIGINT, 'PAINT', 'normal'::selection_type),
		(10009::BIGINT, 'CODE', 'normal'::selection_type),
		(10009::BIGINT, 'BADMINTON', 'normal'::selection_type),
		(10009::BIGINT, 'SERVICE', 'normal'::selection_type),
		(10009::BIGINT, 'PHOTO', 'normal'::selection_type),
		(10010::BIGINT, 'MUN', 'normal'::selection_type),
		(10010::BIGINT, 'DANCE', 'normal'::selection_type),
		(10010::BIGINT, 'GVOL', 'normal'::selection_type),
		(10010::BIGINT, 'PAINT', 'normal'::selection_type),
		(10011::BIGINT, 'SCIRES', 'normal'::selection_type),
		(10011::BIGINT, 'CHESS', 'normal'::selection_type),
		(10011::BIGINT, 'FOOTBALL', 'normal'::selection_type),
		(10012::BIGINT, 'MUN', 'normal'::selection_type),
		(10012::BIGINT, 'MAKER', 'normal'::selection_type),
		(10012::BIGINT, 'BADMINTON', 'normal'::selection_type)
)
INSERT INTO choices (student_id, course_id, period_id, selection_type)
SELECT
	d.student_id,
	d.course_id,
	(
		SELECT cp.period_id
		FROM course_periods cp
		JOIN periods p ON p.id = cp.period_id
		WHERE cp.course_id = d.course_id
		ORDER BY p.ordinal
		LIMIT 1
	),
	d.selection_type
FROM desired d
WHERE NOT EXISTS (
	SELECT 1
	FROM choices ch
	WHERE ch.student_id = d.student_id
		AND ch.course_id = d.course_id
);

COMMIT;
