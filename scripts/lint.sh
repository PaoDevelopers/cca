#!/bin/sh

set -eux

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

golangci-lint run ./...

cd frontend

npm run lint:typescript
npm run lint:eslint
npm run lint:prettier
