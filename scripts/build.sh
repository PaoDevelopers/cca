#!/bin/sh

set -eux

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

which sqlc >/dev/null && sqlc generate || go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate

go build -o cca ./cmd/cca

cd frontend

npm run build
