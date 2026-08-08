-- Fixture for the end-to-end tests. Loaded into a scratch database on
-- top of internal/db/schemas, so it goes through the same constraints
-- and write functions production does.
--
-- No sessions here: they are signed cookies with nothing stored, and
-- the harness mints them with `cca -session`, which is the server's
-- own encoder rather than a second copy of the format.

INSERT INTO categories (id, name) VALUES
	('SPORT', 'Sport'),
	('ART', 'Art');

-- Both windows open now and never close, so the tests act without
-- watching a clock. A closed grade is set up per-test where it is what
-- is under test.
INSERT INTO grades (id, name, opens_at, closes_at,
		max_budgeted_periods, min_distinct_categories, sort_order) VALUES
	('Y9', 'Year 9', now() - interval '1 hour', NULL, 4, 0, 1),
	('Y10', 'Year 10', now() - interval '1 hour', NULL, 4, 0, 2);

INSERT INTO periods (id, name, sort_order) VALUES
	('MON1', 'Monday 1', 1),
	('TUE2', 'Tuesday 2', 2),
	('WED3', 'Wednesday 3', 3);

-- Basketball and Baking both meet in MON1, so they clash; Chess does
-- not clash with either. Basketball seats two, which is what makes the
-- live count observable without a crowd.
INSERT INTO courses (id, name, description, max_students, invite_only,
		teacher, teacher_email, location, term, cost, category_id) VALUES
	('BB', 'Basketball', 'Indoor basketball', 2, FALSE,
		'Mel', 'mel@example.org', 'Gym', 'Season', '', 'SPORT'),
	('BK', 'Baking', 'Cakes and bread', 10, FALSE,
		'Kim', '', 'Kitchen', 'Season', '200 rmb', 'ART'),
	('CH', 'Chess', '', 10, FALSE,
		'Ray', '', 'S206', 'Year', '', 'SPORT');

INSERT INTO course_periods (course_id, period_id) VALUES
	('BB', 'MON1'),
	('BK', 'MON1'),
	('CH', 'TUE2');

-- Student ids are email localparts, as everywhere: the same string the
-- roster carries and a sign-in produces.
INSERT INTO students (id, name, grade_id, legal_sex) VALUES
	('s1001', 'Alice Example', 'Y9', 'F'),
	('s1002', 'Bob Example', 'Y9', 'M');
