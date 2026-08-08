-- Between-season resets. Each clears one section wholesale; sections
-- still referenced by others are refused by the foreign keys, which
-- is what fixes the order an administrator has to work in.
--
-- These are plain DELETEs rather than write functions because they
-- judge nothing: emptying a table cannot leave a violation behind.
--
-- They deliberately do not behave like delete_course, which removes a
-- course's enrollments along with it. That is right for what it is: an
-- administrator naming one course to cancel has decided about that
-- course, and refusing until they had unenrolled every student by hand
-- would make cancelling one mid-season impossible.
--
-- A reset names nothing. It is a sweep, and a sweep that finds
-- enrollments still in the table has been run in the wrong order, so
-- the foreign keys refuse it and the refusal is the message. The two
-- sit next to each other on the same page and read as one operation;
-- they are not, which is why this says so.

-- name: DeleteAllEnrollments :exec
DELETE FROM enrollments;

-- name: DeleteAllStudents :exec
DELETE FROM students;

-- name: DeleteAllCourses :exec
DELETE FROM courses;

-- name: DeleteAllPeriods :exec
DELETE FROM periods;

-- name: DeleteAllGrades :exec
DELETE FROM grades;

-- name: DeleteAllCategories :exec
DELETE FROM categories;

-- Shuts every window that is currently open, by recording that it
-- closed now.
--
-- Not by clearing the bounds: those are configuration an
-- administrator set, often a staggered schedule across grades, and
-- erasing them is unrecoverable. Writing closes_at instead leaves
-- opens_at alone, so the row still says when the window ran, and a
-- grade that has not opened yet — or has already closed — is left
-- untouched rather than being given a spurious closing time.
-- name: CloseOpenWindows :execrows
UPDATE grades
SET closes_at = now()
WHERE opens_at IS NOT NULL
	AND opens_at <= now()
	AND (closes_at IS NULL OR closes_at > now());

-- How many grades could still be acted in. A reset asks this rather
-- than closing them itself: emptying a table is not a licence to
-- rewrite the season's schedule.
-- name: CountOpenWindows :one
SELECT count(*) FROM v_grades WHERE is_open;
