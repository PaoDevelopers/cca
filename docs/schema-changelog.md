# Schema changelog

Every change to `internal/db/schemas/` moves `db.SchemaFingerprint`.
This file records which of those changes also moved
`db.ExpectedSchemaVersion`, and what an administrator has to run
against a live database, since there are no automatic migrations.

## Student refusals judged before the locks

Fingerprint `1947457a…1d52`. Schema version unchanged (1).

`self_enroll` and `self_swap` now run their checks once without locks
before taking them, through the new `judge_self_enrollment`, so a
student refused (a full course, mostly) is turned away at once instead
of queuing behind everyone else on that course's row lock. The checks
run again inside the locks, as before, and that pass decides.
Signatures, error codes and their order are unchanged.

Either binary works with either definition, so this can be applied
before or after deploying, without downtime:

```sh
sed -n "/^-- The student path's judgement/,/^-- An administrator places a set/p" \
	internal/db/schemas/0013_enrollment_writes.sql |
	sed 's/^CREATE FUNCTION/CREATE OR REPLACE FUNCTION/' |
	psql -v ON_ERROR_STOP=1 -1 cca
```

## Catalogue eligibility judged set-wise

Fingerprint `1cd95b6a…f714`. Schema version unchanged (1).

`student_course_violations`, behind `GET /student/api/eligibility`, now
evaluates each rule once over all candidate courses instead of calling
`enrollment_violations` once per course. Its signature and results are
unchanged; on the 180-course fake dataset it runs about 11 times
faster under 16 concurrent clients. `enrollment_violations`, which the
writes use, is untouched.

Either binary works with either definition, so this can be applied
before or after deploying, without downtime, and undone by re-running
the old definition:

```sh
sed -n '/^CREATE FUNCTION student_course_violations(/,$p' \
	internal/db/schemas/0012_enrollment_violations.sql |
	sed 's/^CREATE FUNCTION/CREATE OR REPLACE FUNCTION/' |
	psql -v ON_ERROR_STOP=1 -1 cca
```
