-- This is the 'gender' field from PowerSchool,
-- but it is more accurately described as legal sex.
-- Enum values are display names:
-- they are shown to users and appear in spreadsheets as-is,
-- so there is no renaming layer between storage and presentation.
CREATE TYPE legal_sex AS ENUM ('F', 'M', 'X');

-- The technical key of an entity (category, grade, period, course),
-- spoken by every machine interface
-- (imports, API payloads, violation codes, logs).
-- ':' must stay excluded because violation codes use it as a delimiter.
-- Ill-cased input is rejected, never case-folded.
-- Columns referencing an entity key use this domain too,
-- so the grammar holds even where foreign key enforcement
-- is suspended (bulk loads, restores, NOT VALID constraints).
CREATE DOMAIN entity_id AS TEXT CHECK (VALUE ~ '^[A-Z0-9_-]{1,16}$');

-- A student key: the localpart of the school email address,
-- e.g. 's22537' for a student,
-- or 'runxi.yu' when a teacher or administrator account
-- is enrolled for testing purposes.
-- Stored lowercase;
-- the application lowercases what it receives at the auth boundary.
-- ':' must stay excluded, as in entity_id.
CREATE DOMAIN localpart AS TEXT
	CHECK (VALUE ~ '^[a-z0-9]+(\.[a-z0-9]+)*$' AND length(VALUE) <= 64);

-- The whitespace that counts as whitespace.
--
-- Spelled out by codepoint rather than left to '[[:space:]]', which is
-- the C library's idea of the class and excludes exactly the
-- characters that cause trouble here: U+00A0 and U+202F, the
-- non-breaking spaces, are not "space" to glibc but are to Go's
-- unicode.IsSpace.
--
-- That disagreement is not academic. The application trims cells with
-- TrimSpace on the way in from a spreadsheet, so a name the database
-- accepted with a leading U+00A0 came back from the database's own CSV
-- export as a different name, silently; and a name that was nothing
-- but U+00A0 was accepted, exported, and then rejected the entire file
-- on re-import, with nothing visible in it to find.
--
-- This is exactly the set Go's unicode.IsSpace reports true for, so
-- "the database accepts it" and "TrimSpace leaves it alone" are the
-- same statement, and TestTheDatabaseAndGoAgreeOnWhitespace holds them
-- that way.
CREATE FUNCTION space_chars() RETURNS TEXT
	LANGUAGE sql IMMUTABLE PARALLEL SAFE
	-- tab, newline, vertical tab, form feed, carriage return, space
	RETURN E'\u0009\u000A\u000B\u000C\u000D\u0020'
		-- next line, no-break space, Ogham space mark
		|| E'\u0085\u00A0\u1680'
		-- en quad through hair space
		|| E'\u2000\u2001\u2002\u2003\u2004\u2005'
		|| E'\u2006\u2007\u2008\u2009\u200A'
		-- line separator, paragraph separator
		|| E'\u2028\u2029'
		-- narrow no-break space, medium mathematical, ideographic space
		|| E'\u202F\u205F\u3000';

-- Characters that are invisible but are not whitespace, rejected
-- anywhere in the value rather than only at its edges.
--
-- These defeat the point of the domain from the middle: two course
-- names differing by a zero-width space are two rows nobody can tell
-- apart, and a bidi override in a name reorders the text around it in
-- every list that displays it. None can be typed by accident; they
-- arrive by paste, from a word processor, or on purpose.
--
-- U+200C and U+200D are absent deliberately. The zero-width joiner is
-- how emoji sequences are built and how several scripts are written
-- correctly, so refusing it would reject legitimate names in order to
-- make a point about illegitimate ones.
CREATE FUNCTION invisible_chars() RETURNS TEXT
	LANGUAGE sql IMMUTABLE PARALLEL SAFE
	-- soft hyphen, Arabic letter mark
	RETURN E'\u00AD\u061C'
		-- zero-width space, left-to-right mark, right-to-left mark
		|| E'\u200B\u200E\u200F'
		-- the bidi embeddings and overrides
		|| E'\u202A\u202B\u202C\u202D\u202E'
		-- word joiner and the invisible operators
		|| E'\u2060\u2061\u2062\u2063\u2064'
		-- the bidi isolates, and the byte order mark
		|| E'\u2066\u2067\u2068\u2069\uFEFF'
		-- The line and paragraph separators. They are in space_chars,
		-- so they were trimmed at the edges and passed in the middle —
		-- and [[:cntrl:]] does not catch them, because glibc classes
		-- them Zl and Zp rather than control. U+2028 is a forced line
		-- break in CSS text, so a name carrying one occupies two lines
		-- in every table cell that shows it, which is the one thing
		-- this domain exists to make impossible.
		|| E'\u2028\u2029'
		-- The Hangul fillers and the Mongolian vowel separator. Each
		-- renders as nothing at all and is not whitespace to anybody,
		-- so a name made only of them was accepted, listed as a blank
		-- row, and could not be searched for or told from its
		-- neighbour.
		|| E'\u115F\u1160\u3164\uFFA0\u180E'
		-- The interlinear annotation characters, which are explicitly
		-- not for interchange.
		|| E'\uFFF9\uFFFA\uFFFB'
		-- The tag block, U+E0000 to U+E007F: a complete invisible
		-- alphabet. A name can carry an entire second name in it and
		-- render as nothing. Written as a range because listing 128
		-- codepoints by hand invites one to be missed.
		|| (SELECT string_agg(chr(cp), '')
			FROM generate_series(917504, 917631) AS cp);

-- Whether a value is usable as one line of display text: no control
-- characters, nothing invisible, no whitespace at either end, and
-- short enough to sit in a table cell, a CSV column and a log line.
--
-- The edge test is an equality against btrim rather than two anchored
-- regexes, because being its own trim is the property the application
-- depends on, and writing it that way states the property rather than
-- restating a consequence of it. The same for translate: a value that
-- survives having every invisible character deleted from it contained
-- none.
--
-- 200 characters is far past any real course, teacher, room or term,
-- and far short of a name that pushes every other column off the
-- screen. Prose that legitimately runs long is `description`, which is
-- plain TEXT and not this.
CREATE FUNCTION is_single_line(value TEXT) RETURNS BOOLEAN
	LANGUAGE sql IMMUTABLE PARALLEL SAFE
	RETURN value = btrim(value, space_chars())
		AND value !~ '[[:cntrl:]]'
		AND value = translate(value, invisible_chars(), '')
		AND char_length(value) <= 200;

-- How many malformed elements of one batch are described individually.
--
-- The count in the message is always the true one; this bounds only
-- the list attached to it. Two reasons, and the second is the load
-- bearing one. Nobody reads sixteen thousand rejections — the first
-- few say what is wrong with the file, and the rest say it again. And
-- the rejections were accumulated with jsonb ||, which reserialises
-- everything gathered so far on every append, so a file in which every
-- row is bad cost time quadratic in its length: measured at 0.13 s for
-- a thousand elements, 1.8 s for four thousand, and 29.7 s for sixteen
-- thousand. Thirty seconds is the write timeout, so the single most
-- ordinary administrator mistake — the wrong spreadsheet, or one whose
-- columns are shifted, where every row fails — did not come back as a
-- list of bad rows at all. It came back as "The system is busy or
-- briefly unavailable. Please try again in a moment.", after burning a
-- core and a pool connection for half a minute, and it said the same
-- thing on every retry.
CREATE FUNCTION max_reported_elements() RETURNS INTEGER
	LANGUAGE sql IMMUTABLE PARALLEL SAFE
	RETURN 100;

-- Single-line display text: non-empty, no leading or trailing
-- whitespace of any kind, and no control or invisible characters, so
-- no value can break a table layout, export, or log line, and no value
-- can render as nothing at all.
--
-- It does not make visually identical values impossible, and saying so
-- here would be a claim the domain cannot keep: NBSP and U+3000 differ
-- from a space in the middle of a value, and NFC and NFD spellings of
-- an accented name are different strings. Both are one paste away from
-- a word processor or an IME. Deciding what to do about them is a
-- question about normalising input, not about what a single line is.
CREATE DOMAIN trimmed_text AS TEXT
	CHECK (VALUE <> '' AND is_single_line(VALUE));

-- As trimmed_text, but '' is allowed and means absence.
-- Display prose is never NULL; absence is always ''.
CREATE DOMAIN trimmed_text_opt AS TEXT
	CHECK (is_single_line(VALUE));

-- A count of things that exist in this database: students in a course,
-- periods in a week, categories a grade must span.
--
-- The upper bound is not a guess at a school's size; it is the point
-- past which the value stops surviving the round trip to a browser.
-- Both frontends parse JSON with JSON.parse, which represents integers
-- as doubles, so a count above 2^53 comes back changed — and a card
-- built from the changed value saves the change, silently, or is
-- refused for being out of range and can then never be saved at all.
--
-- A million is a long way past any real count and a long way short of
-- that boundary, so every value the database will accept is one the
-- browser can hold exactly and hand back unaltered.
CREATE DOMAIN count_value AS BIGINT CHECK (VALUE >= 0 AND VALUE <= 1000000);
