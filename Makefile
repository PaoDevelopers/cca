.POSIX:

.PHONY: all
all: cca

ui/node_modules: ui/package.json ui/package-lock.json
	cd ui && npm install
	touch ui/node_modules

.PHONY: sql
sql:
	sqlc generate

.PHONY: ui
ui: ui/node_modules
	cd ui && npm run build

.PHONY: cca
cca: ui sql
	go build -o . ./cmd/cca

# The schema's own tests are Go tests now (internal/db/*_test.go),
# against a scratch database the harness clones per test, so they run
# under check-go with everything else rather than through a separate
# shell runner.
.PHONY: check
check: check-go check-ui check-e2e

.PHONY: check-go
check-go: ui sql
	go test -race $$(go list ./... | grep -v '/ui/node_modules/')
	go vet $$(go list ./... | grep -v '/ui/node_modules/')
	golangci-lint run ./...

.PHONY: check-ui
check-ui: ui/node_modules
	cd ui && npm run check:svelte
	cd ui && npm run check:tsc
	cd ui && npm run check:eslint
	cd ui && npm run check:prettier

.PHONY: check-e2e
check-e2e: cca
	cd ui && npm run check:e2e

.PHONY: clean
clean:
	rm -rf cca ui/dist ui/admin/dist ui/.e2e

.PHONY: distclean
distclean: clean
	rm -rf ui/node_modules

.PHONY: fmt
fmt: ui/node_modules
	cd ui && npm run fmt

.PHONY: update
update: ui/node_modules
	cd ui && npm update --save
