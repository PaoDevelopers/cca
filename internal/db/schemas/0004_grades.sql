-- Grades.
-- The id matches the vocabulary of the PowerSchool student data,
-- so student imports need no mapping;
-- the name carries whichever house style is in fashion
-- ('Year 9', 'Grade 9', ...).
CREATE TABLE grades (
	id entity_id PRIMARY KEY,
	name trimmed_text NOT NULL UNIQUE,

	-- The enrollment window:
	-- students in this grade may act
	-- (add, drop, or swap enrollments, including declining invitations)
	-- if and only if opens_at <= now() < closes_at.
	-- A NULL opens_at means the window is closed;
	-- a NULL closes_at means it never closes once open.
	--
	-- Openness is derived wherever it is read, so there is no state to
	-- repair after downtime, a clock change, or a missed tick: a
	-- student acting one second after closes_at is refused whether or
	-- not anything noticed.
	--
	-- One thing does fire at a bound, and it is deliberately the least
	-- reliable component in the system: internal/web/window_timer.go
	-- arms a single timer at the next boundary across every grade and
	-- broadcasts an invalidation, so a page already open repaints then
	-- rather than at the student's next action. Missing it costs a
	-- stale button, never a wrong answer.
	opens_at TIMESTAMPTZ,
	closes_at TIMESTAMPTZ,
	CHECK (opens_at IS NULL OR closes_at IS NULL OR closes_at > opens_at),

	-- Caps the periods a student in this grade may occupy
	-- with counts_toward_budget enrollments;
	-- NULL means no cap.
	-- The unit is periods, not courses:
	-- a course meeting twice a week counts 2.
	max_budgeted_periods count_value,

	-- Enrollments in scheduled courses must span at least this many
	-- distinct categories.
	-- Like the requirement groups below,
	-- this is reported to the student, not enforced as a transition rule.
	-- 0 means no requirement.
	min_distinct_categories count_value NOT NULL,

	-- Display order.
	-- Reordering is declarative (set_grade_order names the whole
	-- order), so the column carries no invariant beyond being a
	-- number: not gapless, not unique, nothing deferred.
	-- Ties are broken by id wherever the order is read,
	-- so a tie is a cosmetic accident, not a corrupt state,
	-- and the next reorder repairs it.
	sort_order count_value NOT NULL CHECK (sort_order > 0)
);

-- A requirement group names a set of categories
-- and a minimum number of periods
-- a student must occupy across them.
-- To require "at least n periods in total",
-- create a requirement group holding every category.
CREATE TABLE grade_requirement_groups (
	id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	grade_id entity_id NOT NULL REFERENCES grades(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	min_period_count count_value NOT NULL
);

CREATE INDEX grade_requirement_groups_grade_id_idx
	ON grade_requirement_groups (grade_id);

CREATE TABLE grade_requirement_group_categories (
	requirement_category_id BIGINT NOT NULL
		REFERENCES grade_requirement_groups(id)
		ON UPDATE CASCADE ON DELETE CASCADE,
	category_id entity_id NOT NULL REFERENCES categories(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,
	PRIMARY KEY (requirement_category_id, category_id)
);

-- requirement_category_id leads the primary key;
-- category_id is what a category deletion must prove absent.
CREATE INDEX grade_requirement_group_categories_category_id_idx
	ON grade_requirement_group_categories (category_id);
