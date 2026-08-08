-- Constraints declared on tables are invariants:
-- they hold for every actor, always.
-- Rules that depend on who is acting,
-- such as the self-enrollment rules and administrative acceptance
-- of reported violations,
-- are enforced in the write functions defined later.

-- Indexing convention.
-- PostgreSQL indexes the referenced side of a foreign key
-- (it is a key) but never the referencing side,
-- so every referencing column is indexed explicitly here.
-- Two things make that more than hygiene in this schema:
-- entity ids carry ON UPDATE CASCADE, so renaming an id rewrites
-- every referencing row and scans every referencing table;
-- and ON DELETE RESTRICT has to prove absence before any delete.
-- Where the leading column of an existing key already covers the
-- reference, no separate index is created and a comment says so.
-- Indexes beyond that exist only where a read predicate was measured
-- to need one.

-- There are no automatic migrations;
-- administrators diff the schema and migrate by hand.
-- This singleton exists only so the application can refuse to run
-- against an incompatible schema.
CREATE TABLE schema_version (
	singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
	version BIGINT NOT NULL CHECK (version > 0)
);
INSERT INTO schema_version (version) VALUES (1);
