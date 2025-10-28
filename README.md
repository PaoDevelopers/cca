# YK Pao School Co-curricular Activities Selections System

[Main repo](https://git.sr.ht/~runxiyu/cca)\
[Issue tracker](https://todo.sr.ht/~runxiyu/cca)

## Build

You need a recent [Go](https://go.dev) toolchain. [`sqlc`](https://sqlc.dev) is
necessary but will be downloaded and run automatically if absent from `$PATH`.

To build, just run `./build`.

## Configuration and setup

Adapt `cca.scfgs` to your environment.

Note that this service does not have automatic database schema migrations.
Instance administrators are required to run the schema and relevant migrations
themselves.
