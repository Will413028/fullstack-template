# Backend commands. Needs Go (version in go.mod) and Docker (Atlas dev database, integration tests).
# Tool versions are pinned only here; CI runs `make check` too, so local and CI use the same versions.

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
GOVULNCHECK   := go run golang.org/x/vuln/cmd/govulncheck@v1.8.0

# Generating migrations and drift checks use the Atlas standard build: the community build rejects several
# schema.sql files (docs/adr/2026-10-18-go-backend.md D2). The image applies migrations with the community build
# (see Dockerfile). Checksums are the .sha256 files published next to each binary on release.ariga.io.
ATLAS_VERSION := v1.1.3
ATLAS         := bin/atlas
ATLAS_SHA256_darwin-arm64 := b587a8d1c4da7f5891300e06576b0286fd0270b7c531026f019db1558d0706d0
ATLAS_SHA256_linux-amd64  := d2c3675dcf5f88989506e34b3de4d8b76f7fb23d02422c4b5e5dd59fe257f1ab
ATLAS_SHA256_linux-arm64  := 6c898b2aaa0e84e84ecbf4d8f9b74007f16815759069651a718e9bee25319656
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
lint:
	$(GOLANGCI_LINT) run ./...

.PHONY: test
test:
	go test -race ./...

.PHONY: vuln
vuln:
	$(GOVULNCHECK) ./...

.PHONY: check
check: fmt-check lint test vuln

.PHONY: tools
tools: $(ATLAS)

$(ATLAS):
	@sum="$(ATLAS_SHA256_$(PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no Atlas checksum for $(PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://release.ariga.io/atlas/atlas-$(PLATFORM)-$(ATLAS_VERSION)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@

.PHONY: migration
migration: $(ATLAS) ## Generate a migration from the modules' schema.sql: make migration name=add_users
	@test -n "$(name)" || { echo "usage: make migration name=<name>"; exit 1; }
	$(ATLAS) migrate diff $(name) --env dev
