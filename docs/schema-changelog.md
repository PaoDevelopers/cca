# Schema changelog

Every change to `internal/db/schemas/` moves `db.SchemaFingerprint`.
This file records which of those changes also moved
`db.ExpectedSchemaVersion`, and what an administrator has to run
against a live database, since there are no automatic migrations.

## Eligibility rules evaluated over a set of courses

Fingerprint `fdc1aee1…c167`. Schema version unchanged (1).

The five enrollment rules moved into a new function, `course_violations`,
which judges one student against a set of courses in one pass.
`enrollment_violations` and `student_course_violations` keep their
signatures and results and now call it. Nothing else changes: the same
rows come back for every input, only faster — the catalogue read by
about three times, the single-course check used by the writes by about
a third.

Because the signatures are unchanged, the binary does not care whether
this has been applied: an old binary works against the new functions
and a new binary against the old ones. It can be applied before or
after deploying, without downtime, and undone by re-running the old
definitions.

To apply, run the `course_violations`, `enrollment_violations` and
`student_course_violations` definitions from
`internal/db/schemas/0012_enrollment_violations.sql`, in that order, in
one transaction, with `CREATE FUNCTION enrollment_violations` and
`CREATE FUNCTION student_course_violations` changed to
`CREATE OR REPLACE FUNCTION`:

```sh
{
	echo 'BEGIN;'
	sed -n '/^CREATE FUNCTION course_violations(/,$p' \
		internal/db/schemas/0012_enrollment_violations.sql |
		sed 's/^CREATE FUNCTION \(enrollment_violations\|student_course_violations\)(/CREATE OR REPLACE FUNCTION \1(/'
	echo 'COMMIT;'
} | psql -v ON_ERROR_STOP=1 cca
```
