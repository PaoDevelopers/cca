# YK Pao School Co-curricular Activities Selections System

[Main repo](https://github.com/PaoDevelopers/cca)

## Build

You need [Go](https://go.dev) 1.27,
[npm](https://www.npmjs.com/),
and [sqlc](https://docs.sqlc.dev/en/latest/overview/install.html).

Run `make` to build, and `make clean` to remove what it built.

## Checks

On top of the build requirements it needs
[golangci-lint](https://golangci-lint.run),
a PostgreSQL the current user can create databases on,
and a browser in `PATH` for the end-to-end tests
(`chromium`, `chromium-browser` or `google-chrome`).

## Web development

Store a cookie by visiting the backend first,
then you should be able to use the vite development server.

You need to set up a few environment variables
as seen in `ui/dev.ts`.

## Configuration and setup

Adapt `cca.conf` to your environment.

Note that this service
does not have automatic database schema migrations.
Instance administrators run the schema and the migrations themselves.

### Health checks

- `GET /healthz`: is the process alive?
  no database. Restarting is the fix when this fails, and only when
  this fails.
- `GET /readyz`: can it serve right now?

### Reverse proxies

We recommend **not** using reverse proxies.
If you must, make sure they handle WebSocket correctly.

