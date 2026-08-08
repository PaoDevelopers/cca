# YK Pao School Co-curricular Activities Selections System

[Main repo](https://github.com/PaoDevelopers/cca)

## Build

You need [Go](https://go.dev) 1.27,
[npm](https://www.npmjs.com/),
and [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html).

Run `make` to build, and `make clean` to remove what it built.

## Checks

`make check` runs everything: the Go tests (which include the schema's
own, against scratch databases), the frontend's type and lint checks,
and the end-to-end tests. Individual targets are `check-go`,
`check-ui` and `check-e2e`.

On top of the build requirements it needs
[golangci-lint](https://golangci-lint.run),
a PostgreSQL the current user can create databases on,
and a browser in `PATH` for the end-to-end tests
(`chromium`, `chromium-browser` or `google-chrome`; one is never
downloaded, and a missing one is an error rather than a skip).
See `ui/e2e/README` for what those tests do and do not stub out.

## Web development

Store a cookie by visiting the backend first,
then you should be able to use the vite development server.

You need to set up a few environment variables
as seen in `ui/dev.ts`.

## Configuration and setup

Adapt `cca.conf` to your environment. It has no usable default for
`session.key`: generate one with `openssl rand -hex 32` and keep it
out of version control. Sessions are stateless signed cookies, so
rotating that key signs everybody out, which is also the only way to
revoke a session before it expires.

Note that this service does not have automatic database schema
migrations. Instance administrators run the schema and the migrations
themselves; `docs/schema-changelog.md` says what to run and in what
order, and the server refuses to start against a schema version it was
not built for, so a missed migration is a server that does not come up
rather than a query that fails mid-season.

### Health checks

Two endpoints, answering two questions with two different remedies:

- `GET /healthz` — is the process alive? It touches nothing: no pool,
  no database. Restarting is the fix when this fails, and only when
  this fails.
- `GET /readyz` — can it serve right now? It reads through the pool,
  so a database that has stopped answering, or a pool saturated by a
  rush, reports 503. Taking the instance out of rotation is the fix;
  restarting it is not, and restarting during a rush drops every
  websocket and sends the whole school back to reconnect.

Point a supervisor at `/healthz` and a load balancer at `/readyz`.
Pointing a supervisor at `/readyz` is the configuration to avoid.

### Reverse proxies

We recommend **not** using reverse proxies. If you must, make sure they handle
WebSocket correctly.

