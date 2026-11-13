-- One row per student x course,
-- regardless of how many periods the course meets in;
-- a student's occupancy of the weekly grid is derived
-- by joining enrollments with course_periods.
--
-- The two booleans are this row's ongoing policy,
-- and are independent: all four combinations are legal.
-- The old normal/invite/force enum was three of the four cells
-- and could not express "the student's own committed pick"
-- (counts_toward_budget and not student_droppable).
--
-- An enrollment records no provenance:
-- not who created it, not which rules were checked,
-- not whether violations were accepted.
-- Those facts belong to the structured logs.
CREATE TABLE enrollments (
	student_id localpart NOT NULL REFERENCES students(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,
	course_id entity_id NOT NULL REFERENCES courses(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,

	-- May the student drop this enrollment themselves,
	-- while their grade's window is open?
	student_droppable BOOLEAN NOT NULL,

	-- Does this enrollment charge the student's period budget
	-- (grades.max_budgeted_periods)?
	counts_toward_budget BOOLEAN NOT NULL,

	PRIMARY KEY (student_id, course_id)
);

-- student_id needs no index of its own:
-- it leads the primary key.
--
-- course_id is both the unindexed side of a foreign key
-- and the most frequently used predicate in the system:
-- the enrollee count in v_courses (once per course, per course-list
-- read), the capacity rule, and every course-scoped write
-- (placement, removal, deletion, rescheduling, restriction edits)
-- select on it.
-- Without this index the course list is one sequential scan of
-- enrollments per course.
CREATE INDEX enrollments_course_id_idx ON enrollments (course_id);

-- Rows of an enrollment import that an administrator chose to decide
-- later: the student has been emailed about a clash, a full course or
-- the like, and the row waits here, not enrolled, until they answer.
-- Resuming feeds these rows back through the same import check, so a
-- rule that has since stopped applying lets the row straight in.
--
-- Not enrollments: nothing reads these for a seat count, a clash or a
-- budget, and a student never sees them. That is also why deleting a
-- student or a course takes their pending rows with it rather than
-- being refused: a postponed question about a cancelled course has no
-- answer left to wait for.
CREATE TABLE pending_enrollments (
	student_id localpart NOT NULL REFERENCES students(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	course_id entity_id NOT NULL REFERENCES courses(id)
		ON UPDATE CASCADE ON DELETE CASCADE,

	-- The policy the import row stated, kept for when it is decided.
	student_droppable BOOLEAN NOT NULL,
	counts_toward_budget BOOLEAN NOT NULL,

	-- The uploaded file's name: each file waits as its own draft, and
	-- resuming one uploads it again under the same name. A row named
	-- by a later file moves to that file's draft.
	source TEXT NOT NULL,

	-- The row's place in that file. Placement is an ordered batch in
	-- which earlier rows win a course that fills, so resuming replays
	-- the draft in this order, not by id.
	position INTEGER NOT NULL,

	PRIMARY KEY (student_id, course_id)
);

-- student_id leads the primary key; course_id is the referencing side
-- of a foreign key, which a course deletion or rename scans.
CREATE INDEX pending_enrollments_course_id_idx
	ON pending_enrollments (course_id);
