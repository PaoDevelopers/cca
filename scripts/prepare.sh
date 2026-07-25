#!/bin/sh

set -eux

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

cd frontend

npm install
