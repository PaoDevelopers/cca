# Schema changelog

Every change to `internal/db/schemas/` moves `db.SchemaFingerprint`.
This file records which of those changes also moved
`db.ExpectedSchemaVersion`, and what an administrator has to run
against a live database, since there are no automatic migrations.

## Import rows decided later

Fingerprint `b0d4ded5…3b38`. **Schema version 1 → 2.**

New table `pending_enrollments`: rows of an enrollment import left
undecided, or a whole import closed without uploading, waiting as a
draft named after the uploaded file, in the file's row order. An upload writes them in the same
transaction as its placements, and clears any pending row it names.
Clearing the enrollments section clears them too.

The version moves because this binary reads the table and an older
schema does not have it. Each binary refuses the other's version, so
stop cca, apply this, and start the new binary:

```sh
{ sed -n '/^-- Rows of an enrollment import that an administrator chose/,$p' \
	internal/db/schemas/0008_enrollments.sql
  echo 'UPDATE schema_version SET version = 2 WHERE singleton;'
} | psql -v ON_ERROR_STOP=1 -1 cca
```

## Violation payload built in plpgsql

Fingerprint `19d07a7a…3d4a`. Schema version unchanged (1).

`unaccepted_violations` is now `LANGUAGE plpgsql` with the same
signature and body. As a `sql` function it was re-planned on every call
from `judge_self_enrollment` and the admin writes, and planning was
about 90% of its cost: `judge_self_enrollment` drops from 0.60 ms to
0.06 ms per call, and every student write runs it twice. Results are
unchanged.

Either binary works with either definition, so this can be applied
before or after deploying, without downtime:

```sh
sed -n '/^CREATE FUNCTION unaccepted_violations(/,/^\$\$;/p' \
	internal/db/schemas/0013_enrollment_writes.sql |
	sed 's/^CREATE FUNCTION/CREATE OR REPLACE FUNCTION/' |
	psql -v ON_ERROR_STOP=1 -1 cca
```

## Requirement satisfaction counted in one aggregate

Fingerprint `322ca959…dee1`. Schema version unchanged (1).

`v_student_requirements`, behind `GET /admin/api/students/status` and
the requirements in `GET /student/api/user_info`, now counts satisfied
periods in one aggregate over enrollments instead of a lateral per
(student, requirement group). Under a load test of 600 students the
old plan took about 900 ms per administrator refresh whenever the
enrollments statistics were stale, which is the start of every window,
and was about 70% of the database's time; the same run now uses about
a third of the database CPU. Columns and rows are unchanged.

Either binary works with either definition, so this can be applied
before or after deploying, without downtime:

```sh
sed -n '/^CREATE VIEW v_student_requirements AS/,/^) sat ON/p' \
	internal/db/schemas/0011_enrollment_views.sql |
	sed 's/^CREATE VIEW/CREATE OR REPLACE VIEW/' |
	psql -v ON_ERROR_STOP=1 -1 cca
```

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
