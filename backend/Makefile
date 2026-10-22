# Backend commands. Needs Go (version in go.mod) and Docker (Atlas dev database, integration tests).
# Tool versions are pinned here (release binaries, with checksums) and in tools/ (go.mod, package.json,
# requirements.txt, which Dependabot updates); CI runs `make check` too, so local and CI use the same versions.

# golangci-lint is installed from its release binary: the project does not support building it with go run/install.
GOLANGCI_LINT_VERSION := 2.14.0
GOLANGCI_LINT := bin/golangci-lint
GOLANGCI_LINT_SHA256_darwin-arm64 := 5ef5f36a7147e91dc58ef9ef4d11bb7bad5ead0c76eb6c01327a73c641d1dcc3
GOLANGCI_LINT_SHA256_linux-amd64  := ab90aeb7b066f92a33415b638a50fe5344bbb75a0d32ad30cc248d88f81032ab
GOLANGCI_LINT_SHA256_linux-arm64  := ee7ec5f3453d15ddf106fae5a4d6c71737712348a979d1fe9cd52ec7ea299bae
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
OAPI_CODEGEN  := $(call GO_TOOL,oapi-codegen)
OAPI_CONFIG   := internal/httpapi/oapi-codegen.yaml
GEN_OUT       := internal/httpapi/gen/api.gen.go
OASDIFF       := $(call GO_TOOL,oasdiff)
COBERTURA     := $(call GO_TOOL,gocover-cobertura)
GOTESTSUM     := $(call GO_TOOL,gotestsum)
DEADCODE      := $(call GO_TOOL,deadcode)
DIFF_COVER    := pipx run diff-cover==$(shell sed -n 's/^diff-cover==//p' tools/requirements.txt)
# Base for the existing-migrations check; CI passes the PR base or, on push, the previous main commit.
BASE ?= origin/main

# Generating migrations and drift checks use the Atlas standard build: the community build rejects several
# schema.sql files (docs/adr/2026-10-18-go-backend.md D2). The image applies migrations with the community build
# (see Dockerfile). Checksums are the .sha256 files published next to each binary on release.ariga.io.
ATLAS_VERSION := v1.3.4
# Atlas removes binaries published more than 6 months ago from its CDN (atlasgo.io/cli-reference, Distributed
# Binaries); `make atlas-age` fails 30 days before that so the pin is bumped while downloads still work.
ATLAS_RELEASED := 2026-10-14
ATLAS_MAX_AGE_DAYS := 150
ATLAS         := bin/atlas
ATLAS_SHA256_darwin-arm64 := e85bb80abbc20d312e42ce320acc7fe35aa985566aede1d911ee2107d99d7174
ATLAS_SHA256_linux-amd64  := aec7e73a561889836b5fb7d8ba8cc55969c593e53188048d83cdf087f23bc476
ATLAS_SHA256_linux-arm64  := 870fdd78dea732b8d1985caed6e8ea424b84dcf3b5cbb7e1d2c06f1ba09e59e4
# The community build applies migrations: in the image (Dockerfile, same checksums for linux) and in the integration
# tests (internal/platform/dbtest), so tests exercise the build that runs in production.
ATLAS_COMMUNITY := bin/atlas-community
ATLAS_COMMUNITY_SHA256_darwin-arm64 := eba89a0a7ca149f71f1ffe9320e6359a39c032e20926bbd39c7d2d18cc50efb3
ATLAS_COMMUNITY_SHA256_linux-amd64  := 746868e056299897e2da3b7a4b02546b665a7566c53d1fc2efa8636b5fd144f5
ATLAS_COMMUNITY_SHA256_linux-arm64  := 4716df1c7bbb991dee7b3056a61e5c91d4a19b96add12623f87f710f019ed702
PLATFORM := $(shell go env GOOS)-$(shell go env GOARCH)

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
check: fmt-check gen-check lint deadcode test vuln sqlc-diff drift squawk migrations-immutable api-breaking tools-selftest

# Functions that neither the binary nor any test reaches.
.PHONY: deadcode
deadcode:
	./scripts/deadcode.sh $(DEADCODE)

# The git-diff checks and the detecting tools still fail on a planted violation (scripts/selftest-checks.sh).
.PHONY: tools-selftest
tools-selftest: $(SQUAWK)
	./scripts/selftest-checks.sh

.PHONY: gen
gen: ## Regenerate internal/httpapi/gen from api/openapi.yaml
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
	$(OAPI_CODEGEN) -config $(OAPI_CONFIG) api/openapi.yaml > "$$tmp" && cp "$$tmp" $(GEN_OUT)

.PHONY: gen-check
gen-check: ## Fail when internal/httpapi/gen differs from api/openapi.yaml
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
	$(OAPI_CODEGEN) -config $(OAPI_CONFIG) api/openapi.yaml > "$$tmp" || exit 1; \
	cmp -s "$$tmp" $(GEN_OUT) || { echo "$(GEN_OUT) is stale: run make gen"; exit 1; }

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
api-breaking: ## api/openapi.yaml must have no breaking change against BASE
	./scripts/check-openapi-breaking.sh "$(OASDIFF)" $(BASE)

.PHONY: tools
tools: $(ATLAS) $(ATLAS_COMMUNITY) $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	@sum="$(GOLANGCI_LINT_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no golangci-lint checksum for $(PLATFORM)"; exit 1; fi; \
	name=golangci-lint-$(GOLANGCI_LINT_VERSION)-$(PLATFORM); \
	mkdir -p bin && \
	curl -sSfL -o bin/$$name.tar.gz "https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION)/$$name.tar.gz" && \
	echo "$$sum  bin/$$name.tar.gz" | shasum -a 256 -c - && \
	tar -xzf bin/$$name.tar.gz -C bin --strip-components=1 $$name/golangci-lint && \
	rm bin/$$name.tar.gz

$(ATLAS):
	@sum="$(ATLAS_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no Atlas checksum for $(PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://release.ariga.io/atlas/atlas-$(PLATFORM)-$(ATLAS_VERSION)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@

$(ATLAS_COMMUNITY):
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
