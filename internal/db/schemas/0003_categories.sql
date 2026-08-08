-- Categories that courses belong to,
-- such as 'Sport', 'Enrichment', 'Art', and 'Culture' at the SJ campus.
--
-- Categories have no write functions, deliberately:
-- every operation on them is single-statement DML
-- (insert, rename, delete blocked by references),
-- which belongs to the query layer.
-- Deleting a category means first re-pointing its courses
-- and editing any requirement that names it.
CREATE TABLE categories (
	id entity_id PRIMARY KEY,
	name trimmed_text NOT NULL UNIQUE
);
