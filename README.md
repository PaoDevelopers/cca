# YK Pao School Co-curricular Activities Selections System

[![builds.sr.ht status](https://builds.sr.ht/~runxiyu/cca.svg)](https://builds.sr.ht/~runxiyu/cca?)

[Main repo](https://git.sr.ht/~runxiyu/cca)\
[Issue tracker](https://todo.sr.ht/~runxiyu/cca)

## Build

You need a recent [Go](https://go.dev) toolchain and
[npm](https://www.npmjs.com/). [`sqlc`](https://sqlc.dev) is necessary but will
be downloaded and run automatically if absent from `$PATH`.

To install NPM packages, run `./prepare`.

To build, just run `./build`.

To lint, just run `./lint`.

The frontend is one React and shadcn application. Vite produces a shared build
that the Go service exposes at `/student/` and `/admin/`; both application areas
use the versioned JSON API below `/api/v1/`.

For local frontend development, run the Go service and then:

```sh
cd frontend
CCA_API_TARGET=http://127.0.0.1:8192 npm run dev
```

Vite proxies `/api` (including WebSocket upgrades) to `CCA_API_TARGET`.

### Local Docker stack

The local Docker stack builds the React frontend and Go service, starts an
isolated PostgreSQL database, and loads `schema.sql` plus `dev/mock_data.sql` on
the first run:

```sh
docker compose up -d --build
```

Open `http://127.0.0.1:8192/test-login`. The app reads test-authentication
settings from the local `cca.scfgs`, which is mounted read-only and is not
copied into the image. Stop the stack with `docker compose down`; add `-v` only
when the local PostgreSQL data should also be deleted.

## Configuration and setup

Adapt `cca.scfgs` to your environment.

### Test authentication

Test authentication is disabled by default and replaces the external OIDC flow
while it is enabled:

```scfg
test_auth {
	enabled true
	allow_remote false
	access_key ""
}
```

With `allow_remote false`, `/test-login` is available only when both the browser
host and connection are loopback (`localhost`, `127.0.0.1`, or `::1`). Student
login accepts an existing database student ID; administrator login accepts only
a username already present in the `admins` configuration block. Test-mode
sessions use the same database-backed session mechanism as normal login. When
`access_key` is non-empty, every test login must provide it, including a local
login.

Remote test authentication must be explicitly enabled with `allow_remote true`
and requires an `access_key` of at least 16 characters. Never enable it in
production. The access key is entered on the test login page and must not be
committed to source control.

Note that this service does not have automatic database schema migrations.
Instance administrators are required to run the schema and relevant migrations
themselves.

For a new database, apply [`schema.sql`](schema.sql). To upgrade a schema-version
1 database, back it up and apply the single atomic
[`migrations/002_multi_periods.sql`](migrations/002_multi_periods.sql) migration.
Run it in a maintenance window: it takes `ACCESS EXCLUSIVE` locks while keys,
reference data, and triggers are replaced.

Before the maintenance window, audit the production labels with
`SELECT DISTINCT period FROM courses ORDER BY period;`. Alias matching is exact;
an assigned label not listed below makes the migration fail and roll back rather
than guessing or dropping a course schedule.

Schema version 2 has exactly 16 immutable timetable slots: Monday through
Thursday, CCA 1 through CCA 4. A CCA may occupy one or more of those slots
through `course_periods`; selecting that CCA reserves all of them. The database
rejects any selection that intersects an existing selection for the student.
Every committed course must have at least one slot; a deferred database
constraint still allows the course and its slots to be created in one
transaction. Once a course has selections, its period assignments are locked
until those selections are reconciled, preventing a schedule edit from silently
creating clashes.

Migration 002 maps the exact v1 aliases `Mon 1` through `Thu 4` to their full
names. Legacy compound aliases `MW1`–`MW4` and `Monday/Wednesday CCA 1`–`4`
expand to both Monday and Wednesday; `TT1`–`TT4` and
`Tuesday/Thursday CCA 1`–`4` expand to both Tuesday and Thursday. It
deliberately aborts if an occupied slot has any unsupported label (including
Friday), or if expansion would double-book an existing student, so no course
assignment is silently discarded. Unsupported unoccupied labels are removed.

Selection correctness is database-owned. PostgreSQL locks the student row and
then the course row before enforcing schedule conflicts, grade windows,
eligibility, own-choice limits, and capacity. Administrative imports and batch
changes call one SQL function that pre-locks every affected student in ID order,
then every affected course in ID order, avoiding cross-batch deadlocks. The
student catalogue and requirement progress are also calculated in SQL from a
shared repeatable-read snapshot. WebSocket count messages update only the named
course in React; they do not cause every connected student to reload the whole
catalogue.

### Reverse proxies

We recommend **not** using reverse proxies. If you must, make sure they handle
WebSocket correctly, preserve the public `Host`, and overwrite (rather than
append to) `X-Forwarded-Proto`.
