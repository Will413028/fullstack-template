# Backend commands. Needs Go (version in go.mod) and Docker (Atlas dev database, integration tests).
# Tool versions are pinned here (release binaries, with checksums) and in tools/ (go.mod, package.json,
# requirements.txt, which Dependabot updates); CI runs `make check` too, so local and CI use the same versions.

# golangci-lint is installed from its release binary: the project does not support building it with go run/install.
GOLANGCI_LINT_VERSION := 2.15.0
GOLANGCI_LINT := bin/golangci-lint
GOLANGCI_LINT_SHA256_darwin-arm64 := d1e409481a8ca846c4bc97e781eb5cb1bd5c23353a0b3977a1cbda1b1b6447d5
GOLANGCI_LINT_SHA256_linux-amd64  := 9b7fe6a6bf6334b98decf8d86b11415ea7889afdabdd1959fdce23af2a9ebcea
GOLANGCI_LINT_SHA256_linux-arm64  := b4d8f17068b35ee6060cc2f69a16fb48740d706fbba81661c318649c114a6998
# Go-built dev tools share one module, tools/go.mod (its own go.sum and build list), so they never change the
# application's go.mod or the production binary. They are dependencies like any other: Dependabot bumps them, osv-scanner
# scans them, and a vulnerable dependency of a tool is upgraded there. gen-check, sqlc-diff and tools-selftest show
# that an upgrade did not change what a tool produces or detects. Add a tool with `cd tools && go get -tool <pkg>@<v>`.
GO_TOOL        = go tool -modfile=tools/go.mod $(1)
GOVULNCHECK   := $(call GO_TOOL,govulncheck)
# sqlc builds with cgo (libpg_query); the first run compiles it, later runs use the Go build cache.
SQLC          := $(call GO_TOOL,sqlc)
# squawk and diff-cover are pinned in tools/package.json and tools/requirements.txt, which Dependabot reads; the
# release binaries above and Atlas below are checked weekly by scripts/tool_versions.py (tool-versions workflow).
SQUAWK        := tools/node_modules/.bin/squawk
# buf is pinned in the root Makefile (make bin/buf), which serves the frontend's generation too.
BUF           := ../bin/buf
COBERTURA     := $(call GO_TOOL,gocover-cobertura)
GOTESTSUM     := $(call GO_TOOL,gotestsum)
DEADCODE      := $(call GO_TOOL,deadcode)
DIFF_COVER    := pipx run diff-cover==$(shell sed -n 's/^diff-cover==//p' tools/requirements.txt)
# Base for the existing-migrations check; CI passes the PR base or, on push, the previous main commit.
BASE ?= origin/main

# Generating migrations and drift checks use the Atlas standard build: the community build rejects several
# schema.sql files (docs/adr/2026-10-18-go-backend.md D2). The image applies migrations with the community build
# (see Dockerfile). Checksums are the .sha256 files published next to each binary on release.ariga.io.
ATLAS_VERSION := v1.3.5
# Atlas removes binaries published more than 6 months ago from its CDN (atlasgo.io/cli-reference, Distributed
# Binaries); `make atlas-age` fails 30 days before that so the pin is bumped while downloads still work.
ATLAS_RELEASED := 2026-10-21
ATLAS_MAX_AGE_DAYS := 150
ATLAS         := bin/atlas
ATLAS_SHA256_darwin-arm64 := c5d7056be2420471c7e167ee9c459e3cf362d8eb7c9ef13fdc5cebac6ec77f75
ATLAS_SHA256_linux-amd64  := 41d2a3c77fab370aead88b9a9faf2bab24d22e3e5728dc2dd7f87404adbce048
ATLAS_SHA256_linux-arm64  := bcdb097755995ef304a08635d38b65e9c2636489f45d0f7c04f2316bbd888ba9
# The community build applies migrations: in the image (Dockerfile, same checksums for linux) and in the integration
# tests (internal/platform/dbtest), so tests exercise the build that runs in production.
ATLAS_COMMUNITY := bin/atlas-community
ATLAS_COMMUNITY_SHA256_darwin-arm64 := b69dd05c33cd1ee1f9a21eb4e71e48f44df325847d66c38f8d81ba0a1aaaafd8
ATLAS_COMMUNITY_SHA256_linux-amd64  := bfc65a52d3b745178c619a314052580861a385c364ae5abece174286055d525a
ATLAS_COMMUNITY_SHA256_linux-arm64  := e934d1649e8e110c934db4dd59ae6fcbec0d2354fef273e4444cb009443a91a3
PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)
# The binaries keep stable paths (docs and lefthook call bin/atlas, bin/golangci-lint) and depend on an empty stamp
# named after the pinned version: after a bump (or a checkout of an older pin) that stamp is missing, so make creates
# it, deleting the tool's other stamps, and the stamp being newer than the binary downloads the pinned version again.
# Without it an existing binary would never be replaced.
GOLANGCI_LINT_STAMP   := bin/.golangci-lint-$(GOLANGCI_LINT_VERSION)
ATLAS_STAMP           := bin/.atlas-$(ATLAS_VERSION)
ATLAS_COMMUNITY_STAMP := bin/.atlas-community-$(ATLAS_VERSION)

.PHONY: run
run: ## Start serve with backend/.env (format in .env-example)
	@set -a && . ./.env && set +a && go run ./cmd/app serve

.PHONY: build
build:
	CGO_ENABLED=0 go build -trimpath -o bin/app ./cmd/app

.PHONY: fmt
fmt:
	gofmt -w .

.PHONY: fmt-check
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "needs gofmt: $$out"; exit 1; fi

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

.PHONY: test
# test-report.json (go test -json) feeds the executed-test count lock in scripts/check_locks.py at the repo root.
# -shuffle=on runs tests in a random order so one that relies on another's leftovers fails; each package prints its
# seed (-test.shuffle N), and -shuffle=N replays that order.
test: $(ATLAS_COMMUNITY) ## Needs Docker: integration tests start PostgreSQL with testcontainers
	$(GOTESTSUM) --format pkgname-and-test-fails --jsonfile test-report.json -- -race -shuffle=on -coverpkg=./... -coverprofile=coverage.out ./...

# Reported, not gated (AGENTS.md Verification). -coverpkg above counts code exercised from other packages' tests.
# COVER_BASE (a ref) adds changed-line coverage against it; CI sets it on pull requests.
.PHONY: coverage
coverage: ## Markdown coverage report from the last make test
	./scripts/coverage-report.sh "$(COBERTURA)" "$(DIFF_COVER)" $(COVER_BASE)

.PHONY: vuln
vuln:
	$(GOVULNCHECK) ./...

.PHONY: check
check: fmt-check gen-check buf-lint lint deadcode test vuln sqlc-diff drift squawk migrations-immutable api-breaking tools-selftest

# Functions that neither the binary nor any test reaches.
.PHONY: deadcode
deadcode:
	./scripts/deadcode.sh $(DEADCODE)

# The git-diff checks and the detecting tools still fail on a planted violation (scripts/selftest-checks.sh).
.PHONY: tools-selftest
tools-selftest: $(SQUAWK) $(BUF)
	./scripts/selftest-checks.sh $(BUF)

# The Connect code: internal/gen from the contract in ../api (buf.gen.yaml), and the test services' code from
# internal/httpapi/rpc/testdata, a buf module of its own that is not part of the contract. The plugins are
# tools/go.mod's; each buf.gen.yaml cleans its output directory, so a deleted proto leaves no stale file.
GEN_DIRS := internal/gen internal/httpapi/rpc/testdata/gen

.PHONY: gen
gen: $(BUF) ## Regenerate the Connect code from ../api and the test services
	$(BUF) generate --template buf.gen.yaml
	cd internal/httpapi/rpc/testdata && $(abspath $(BUF)) generate

# buf writes in place, so the check regenerates and compares the work tree with the index: a changed, deleted or new
# (untracked) file under GEN_DIRS fails. It compares with the index, not HEAD: a regenerated file that is staged but
# not committed passes.
.PHONY: gen-check
gen-check: gen ## Fail when the generated Connect code differs from ../api and the test services
	@out="$$(git diff --name-status -- $(GEN_DIRS); git ls-files --others --exclude-standard -- $(GEN_DIRS))"; \
	if [ -n "$$out" ]; then echo "$$out"; echo "generated Connect code is stale: run make gen and commit it"; exit 1; fi

.PHONY: buf-lint
buf-lint: $(BUF) ## buf lint over the contract and the test services
	$(BUF) lint ../api
	$(BUF) lint internal/httpapi/rpc/testdata

.PHONY: sqlc
sqlc: ## Regenerate each module's sqlc code from its sqlc.yaml
	@for f in internal/modules/*/sqlc.yaml; do \
		[ -e "$$f" ] || { echo "no module sqlc.yaml"; break; }; \
		echo "sqlc generate -f $$f"; $(SQLC) generate -f "$$f" || exit 1; \
	done

.PHONY: sqlc-diff
sqlc-diff: ## Fail when generated sqlc code differs from its schema and queries
	@for f in internal/modules/*/sqlc.yaml; do \
		[ -e "$$f" ] || { echo "no module sqlc.yaml, skipping sqlc diff"; break; }; \
		echo "sqlc diff -f $$f"; $(SQLC) diff -f "$$f" || { echo "generated code is stale: run make sqlc"; exit 1; }; \
	done

.PHONY: drift
drift: $(ATLAS) ## Needs Docker: schema.sql files and migrations/ must match
	./scripts/check-schema-drift.sh $(ATLAS)

.PHONY: squawk
squawk: $(SQUAWK) ## Lock and safety checks for migrations (needs Node)
	./scripts/squawk.sh $(SQUAWK)

# --ignore-scripts: squawk-cli ships its binary as a platform package and runs no install script.
$(SQUAWK): tools/package-lock.json
	npm ci --prefix tools --ignore-scripts --no-audit --no-fund
	@touch $@

.PHONY: migrations-immutable
migrations-immutable: ## Migrations already on BASE must not change
	./scripts/check-migrations-immutable.sh $(BASE)

.PHONY: api-breaking
api-breaking: $(BUF) ## The contract in ../api must have no breaking change against BASE (rules in ../api/buf.yaml)
	../scripts/check-buf-breaking.sh $(BUF) $(BASE)

.PHONY: tools
tools: $(ATLAS) $(ATLAS_COMMUNITY) $(GOLANGCI_LINT) $(BUF)

# The root Makefile is the only download point and checksum list; its stamp replaces the binary after a bump.
.PHONY: $(BUF)
$(BUF):
	@$(MAKE) --no-print-directory -C .. bin/buf

$(GOLANGCI_LINT_STAMP):
	@mkdir -p bin && rm -f bin/.golangci-lint-* && touch $@

# bin/.atlas-v*, not bin/.atlas-*: that would also delete the community build's stamp.
$(ATLAS_STAMP):
	@mkdir -p bin && rm -f bin/.atlas-v* && touch $@

$(ATLAS_COMMUNITY_STAMP):
	@mkdir -p bin && rm -f bin/.atlas-community-v* && touch $@

$(GOLANGCI_LINT): $(GOLANGCI_LINT_STAMP)
	@sum="$(GOLANGCI_LINT_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no golangci-lint checksum for $(PLATFORM)"; exit 1; fi; \
	name=golangci-lint-$(GOLANGCI_LINT_VERSION)-$(PLATFORM); \
	mkdir -p bin && \
	curl -sSfL -o bin/$$name.tar.gz "https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION)/$$name.tar.gz" && \
	echo "$$sum  bin/$$name.tar.gz" | shasum -a 256 -c - && \
	tar -xzOf bin/$$name.tar.gz $$name/golangci-lint > $@.tmp && \
	chmod 0755 $@.tmp && mv $@.tmp $@ && rm bin/$$name.tar.gz

$(ATLAS): $(ATLAS_STAMP)
	@sum="$(ATLAS_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no Atlas checksum for $(PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://release.ariga.io/atlas/atlas-$(PLATFORM)-$(ATLAS_VERSION)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@

$(ATLAS_COMMUNITY): $(ATLAS_COMMUNITY_STAMP)
	@sum="$(ATLAS_COMMUNITY_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no Atlas community checksum for $(PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://release.ariga.io/atlas/atlas-community-$(PLATFORM)-$(ATLAS_VERSION)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@

.PHONY: atlas-age
atlas-age:
	@python3 -I -c 'import datetime as d,sys; age=(d.date.today()-d.date.fromisoformat("$(ATLAS_RELEASED)")).days; \
	sys.exit(0) if age <= $(ATLAS_MAX_AGE_DAYS) else sys.exit(f"Atlas $(ATLAS_VERSION) was released {age} days ago; its binaries leave the CDN at 180 days. Bump ATLAS_VERSION, the checksums here and in Dockerfile, and ATLAS_RELEASED.")'

.PHONY: migration
migration: $(ATLAS) ## Generate a migration from the modules' schema.sql: make migration name=add_users
	@test -n "$(name)" || { echo "usage: make migration name=<name>"; exit 1; }
	$(ATLAS) migrate diff $(name) --env dev
