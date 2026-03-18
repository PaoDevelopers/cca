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

## Configuration and setup

Adapt `cca.scfgs` to your environment.

Note that this service does not have automatic database schema migrations.
Instance administrators are required to run the schema and relevant migrations
themselves.

### Reverse proxies

We recommend **not** using reverse proxies. If you must, make sure they handle
WebSocket correctly.

