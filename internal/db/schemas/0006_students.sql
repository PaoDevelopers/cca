-- Students authenticate via OpenID Connect;
-- sessions are stateless signed cookies,
-- so no session state is stored.
-- Administrators are a config-file allowlist over the same OIDC,
-- and have no table at all.
CREATE TABLE students (
	-- See the localpart domain.
	id localpart PRIMARY KEY,
	name trimmed_text NOT NULL,
	grade_id entity_id NOT NULL REFERENCES grades(id)
		ON UPDATE CASCADE ON DELETE RESTRICT,
	legal_sex legal_sex NOT NULL
);

-- Also the read predicate of set_max_budgeted_periods,
-- which locks and re-judges a grade's students.
CREATE INDEX students_grade_id_idx ON students (grade_id);
