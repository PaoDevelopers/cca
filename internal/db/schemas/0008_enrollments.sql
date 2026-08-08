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
