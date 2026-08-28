-- Course ids are the CCA department's own technical vocabulary,
-- used throughout to name courses precisely.
CREATE TABLE courses (
	id entity_id PRIMARY KEY,
	-- Deliberately not unique: similar names are why ids exist.
	name trimmed_text NOT NULL,
	-- Multi-line prose; '' means none.
	description TEXT NOT NULL,
	-- NULL means no cap: the course takes everyone who signs up.
	-- Distinct from 0, which is a real cap that admits nobody, and
	-- the two were conflated as long as this was NOT NULL -- a
	-- department with no cap in mind wrote 0, and the capacity rule
	-- then refused every enrollment. The rules read the absence
	-- rather than testing for it: current_students >= NULL is NULL,
	-- so an uncapped course simply never reports being full.
	max_students count_value,
	-- Invite-only courses can only be entered through an administrator,
	-- whose input constitutes the required sanction;
	-- students may not enroll themselves.
	invite_only BOOLEAN NOT NULL,
	-- '' means unlisted.
	teacher trimmed_text_opt NOT NULL,
	-- When set, students see the teacher's name as a mailto link.
	-- '' means none.
	teacher_email trimmed_text_opt NOT NULL,
	-- '' means unspecified.
	location trimmed_text_opt NOT NULL,
	-- A label ('Season', 'Semester', ...) rather than a scoping
	-- mechanism: the database holds one season at a time,
	-- so the term does not restrict enrollment or clash checks.
	-- The software never acts on its value;
	-- administrators filter and bulk-delete by it at rollover,
	-- and the PowerSchool export emits it as-is.
	-- '' means unlabelled: a department that does not divide its
	-- season into terms leaves the column empty, and requiring it
	-- rejected whole spreadsheets over a label nothing reads.
	term trimmed_text_opt NOT NULL,
	-- Free text ('Student Paid', '600 rmb', ...) rather than a number:
	-- the source data is prose and is only ever displayed.
	-- '' means none.
	cost trimmed_text_opt NOT NULL,
	category_id entity_id NOT NULL REFERENCES categories(id)
		ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE INDEX courses_category_id_idx ON courses (category_id);

-- A course may meet in any number of periods,
-- including zero while it is still being scheduled.
CREATE TABLE course_periods (
	course_id entity_id NOT NULL REFERENCES courses(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	period_id entity_id NOT NULL REFERENCES periods(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,
	PRIMARY KEY (course_id, period_id)
);

-- course_id leads the primary key;
-- period_id is what a period deletion must prove absent.
CREATE INDEX course_periods_period_id_idx ON course_periods (period_id);

-- Fit restrictions.
-- An empty list means the course is unrestricted on that axis.
CREATE TABLE course_allowed_legal_sexes (
	course_id entity_id NOT NULL REFERENCES courses(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	legal_sex legal_sex NOT NULL,
	PRIMARY KEY (course_id, legal_sex)
);

CREATE TABLE course_allowed_grades (
	course_id entity_id NOT NULL REFERENCES courses(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	grade_id entity_id NOT NULL REFERENCES grades(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,
	PRIMARY KEY (course_id, grade_id)
);

CREATE INDEX course_allowed_grades_grade_id_idx
	ON course_allowed_grades (grade_id);

-- course_allowed_legal_sexes needs no extra index:
-- its only foreign key is course_id, which leads its primary key,
-- and legal_sex is an enum, not a reference.
