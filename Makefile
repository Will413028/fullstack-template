COMPOSE = docker compose
COMPOSE_DEV = $(COMPOSE) -f docker-compose.yml -f docker-compose.dev.yml

# --- Stack ---

.PHONY: up
up:
	$(COMPOSE) up -d
	@echo "\nStack started: the API at http://localhost/api (pages are served by Vercel; Caddy needs X-Origin-Secret)"

.PHONY: dev
dev:
	@echo "Dev stack: the API at http://localhost:8000; run 'pnpm dev' in frontend/ for http://localhost:3000"
	$(COMPOSE_DEV) up --build --watch

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: down-v
down-v:
	$(COMPOSE) down -v

.PHONY: build
build:
	$(COMPOSE) build

.PHONY: restart
restart:
	$(COMPOSE) restart

# --- Logs ---

.PHONY: logs
logs:
	$(COMPOSE) logs -f

.PHONY: logs-backend
logs-backend:
	$(COMPOSE) logs -f backend

.PHONY: logs-caddy
logs-caddy:
	$(COMPOSE) logs -f caddy

.PHONY: logs-postgres
logs-postgres:
	$(COMPOSE) logs -f postgres

# --- Status ---

.PHONY: status
status:
	$(COMPOSE) ps

# --- Database ---

.PHONY: migration
migration:
	$(COMPOSE) build backend
	$(COMPOSE) run --rm migrate

# Generates backend/migrations/<timestamp>_<MSG>.sql from the modules' schema.sql (needs Docker);
# apply it with `make migration` (make dev does not re-run migrate on changes)
.PHONY: generate-migration
generate-migration:
	$(MAKE) -C backend migration name="$(MSG)"

# --- Seed ---

# Creates the admin account in the running stack (app seed-admin; an existing account is left unchanged):
#   make seed ADMIN_ACCOUNT=admin
# The password is read from the terminal without echo, or from stdin when it is piped, and reaches the container on
# its stdin (ADMIN_PASSWORD_FILE=/dev/stdin): never on a command line, in an environment variable or in .env.
.PHONY: seed
seed:
	@test -n "$(ADMIN_ACCOUNT)" || { echo "usage: make seed ADMIN_ACCOUNT=<name> (the password is read from the terminal or stdin)"; exit 1; }
	@if [ -t 0 ]; then \
		trap 'stty echo' EXIT INT TERM; printf 'Password for %s: ' "$(ADMIN_ACCOUNT)" >&2; stty -echo; \
		IFS= read -r pw; stty echo; echo >&2; \
	else IFS= read -r pw; fi; \
	printf '%s' "$$pw" | $(COMPOSE) exec -T -e ADMIN_ACCOUNT="$(ADMIN_ACCOUNT)" -e ADMIN_PASSWORD_FILE=/dev/stdin \
		backend /usr/local/bin/app seed-admin

# The way back when every admin is disabled, locked out or has lost the password (app recover-admin): sets a new
# password for an existing admin, enables it, signs it out everywhere and clears its sign-in throttle. The password is
# read like make seed's.
#   make recover-admin ADMIN_ACCOUNT=admin
.PHONY: recover-admin
recover-admin:
	@test -n "$(ADMIN_ACCOUNT)" || { echo "usage: make recover-admin ADMIN_ACCOUNT=<name> (the new password is read from the terminal or stdin)"; exit 1; }
	@if [ -t 0 ]; then \
		trap 'stty echo' EXIT INT TERM; printf 'New password for %s: ' "$(ADMIN_ACCOUNT)" >&2; stty -echo; \
		IFS= read -r pw; stty echo; echo >&2; \
	else IFS= read -r pw; fi; \
	printf '%s' "$$pw" | $(COMPOSE) exec -T -e ADMIN_ACCOUNT="$(ADMIN_ACCOUNT)" -e ADMIN_PASSWORD_FILE=/dev/stdin \
		backend /usr/local/bin/app recover-admin

# --- Rename ---

# Renames images, packages, database names, branding and the Go module path (default
# github.com/Will413028/<NAME>/backend; MODULE= overrides it), then checks that the backend builds.
.PHONY: rename
rename:
ifndef NAME
	$(error NAME is required. Usage: make rename NAME=my-project [MODULE=example.com/me/my-project/backend])
endif
	@python3 scripts/rename.py "$(NAME)" $(if $(MODULE),--module "$(MODULE)")

.PHONY: init-env
init-env:
	@python3 scripts/init_env.py

# Deletes the database volume (all local data), starts the stack once the backend is ready, and seeds the admin.
.PHONY: reset-db
reset-db:
	@test -n "$(ADMIN_ACCOUNT)" || { echo "usage: make reset-db ADMIN_ACCOUNT=<name> (deletes all local data)"; exit 1; }
	@echo "Stopping stack and clearing database volumes..."
	$(COMPOSE) down -v
	@echo "Starting the stack; migrate runs before the backend, which must report ready..."
	$(COMPOSE) up -d --wait backend
	@$(MAKE) --no-print-directory seed ADMIN_ACCOUNT="$(ADMIN_ACCOUNT)"
	@echo "Database has been reset and seeded."

# --- Checks ---

# hadolint is installed from its release binary; the checksums are the release's checksums.sha256.
HADOLINT_VERSION := 2.15.1
HADOLINT := bin/hadolint
HADOLINT_SHA256_macos-arm64  := 5c09f3213f8e40406abe048233d985eebef336d4a6a20021be47fadb6cf480a2
HADOLINT_SHA256_linux-x86_64 := c7187db94eeeeca956519a6af171adc31453941a1e777961f6e680f697c8c507
HADOLINT_SHA256_linux-arm64  := f6198ef8090f404dbb771abfee086eb8c48ac177f30da7fd3510aca35b344b5d
HADOLINT_PLATFORM := $(subst darwin,macos,$(shell uname -s | tr A-Z a-z))-$(subst aarch64,arm64,$(shell uname -m))
# A stamp named after the pinned version makes a bump replace an existing binary (same as backend/Makefile's tools).
HADOLINT_STAMP := bin/.hadolint-$(HADOLINT_VERSION)

# Every finding fails, infos included (hadolint's default threshold).
.PHONY: lint-docker
lint-docker: $(HADOLINT)
	$(HADOLINT) backend/Dockerfile

$(HADOLINT_STAMP):
	@mkdir -p bin && rm -f bin/.hadolint-* && touch $@

$(HADOLINT): $(HADOLINT_STAMP)
	@sum="$(HADOLINT_SHA256_$(HADOLINT_PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no hadolint checksum for $(HADOLINT_PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://github.com/hadolint/hadolint/releases/download/v$(HADOLINT_VERSION)/hadolint-$(HADOLINT_PLATFORM)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@

# buf (lint, breaking and generation for the contract in api/) is installed from its release binary like hadolint;
# the checksums are the release's sha256.txt. This is the only download: backend/Makefile, the frontend's gen:api
# scripts, CI and lefthook all run `make bin/buf` and then bin/buf, so one version and one checksum list serve them.
BUF_VERSION := 1.75.0
BUF := bin/buf
BUF_SHA256_Darwin-arm64  := f71b009ebd8530b12fd85b060c6c245e15d6e5110ccc8da48d8b6fa71e59330b
BUF_SHA256_Linux-x86_64  := fad1e38d6909bbb583a4da81d8c02a01046e2cd663bde7e337dd24277393764b
BUF_SHA256_Linux-aarch64 := 00eb190aa82df17f393e05b04889cc60a0b13b3259d336e21a87d5f23ab4fd26
BUF_PLATFORM := $(shell uname -s)-$(shell uname -m)
BUF_STAMP := bin/.buf-$(BUF_VERSION)

$(BUF_STAMP):
	@mkdir -p bin && rm -f bin/.buf-* && touch $@

$(BUF): $(BUF_STAMP)
	@sum="$(BUF_SHA256_$(BUF_PLATFORM))"; \
	if [ -z "$$sum" ]; then echo "no buf checksum for $(BUF_PLATFORM)"; exit 1; fi; \
	mkdir -p bin && \
	curl -sSfL -o $@.tmp "https://github.com/bufbuild/buf/releases/download/v$(BUF_VERSION)/buf-$(BUF_PLATFORM)" && \
	echo "$$sum  $@.tmp" | shasum -a 256 -c - && \
	chmod 0755 $@.tmp && mv $@.tmp $@
