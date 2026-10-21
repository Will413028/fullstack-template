# Fullstack Template

Production-ready fullstack monorepo with a Go backend, Next.js frontend, and Docker Compose deployment behind Caddy on one origin.

## Verification

- Backend tests are Go integration tests (testcontainers, PostgreSQL 18), frontend tests use `vitest`; a task is complete only when `make -C backend test` (backend) and `pnpm test` (frontend) pass. CI also runs `make -C backend check` (lint, generated-code, schema and contract checks); run it before pushing backend changes.
- `git push` runs CI's checks first through the lefthook pre-push hook (all but the compose stack job); it needs `actionlint`, `zizmor`, `gitleaks` and `osv-scanner` installed. Do not skip it with `--no-verify`.
- Cover observable behavior, failure conditions, and regressions; do not add tests per module or function when existing tests already cover the behavior. Docs-only or formatting changes need only the relevant static checks.
- Coverage is reported, not gated: CI prints backend total coverage and, on pull requests, changed-line coverage (`diff-cover`) in the job summary; review uncovered changed lines rather than chasing a total percentage. Frontend covers critical paths.

## Constraints

- The schema comes only from Atlas: edit the module's `schema.sql` and generate the migration with `make generate-migration MSG=<name>`; never hand-write DDL (a pure data backfill with `atlas migrate new` is the one exception). A migration merged into `main` never changes; add a new one.
- `make down-v` deletes the Postgres volume and all local data.
- Never commit `.env` or `.secrets/`; add new variables to `.env-example`.
- Agents do not add or upgrade dependencies on their own: list each package and version, confirm the package exists and was not registered recently (deps.dev shows publish dates), and wait for approval before installing.
- Fix known vulnerabilities (`osv-scanner` over every lockfile, `pnpm audit`, `govulncheck`, Dependabot alerts; the Security workflow rescans `main` every Monday) by severity: critical within 7 days, high within 30 days, medium within 90 days, low at the next routine upgrade.

## Monorepo Structure

```
├── backend/                # Go (modular monolith, pgx + sqlc, Atlas), contract in backend/api/openapi.yaml
├── frontend/               # Next.js 16 (App Router, TypeScript, Tailwind, shadcn/ui)
├── deploy/                 # Caddyfile, PostgreSQL initdb scripts (DB roles)
├── docker-compose.yml      # Production stack (postgres + migrate + backend + frontend + caddy)
├── docker-compose.dev.yml  # Dev overrides (compose watch, frontend dev server)
├── Makefile                # Docker Compose shortcuts
├── scripts/                # Repo tooling (rename, init_env)
└── .env-example            # Environment variable template
```

## Quick Reference

```bash
# Docker Compose (from root)
make init-env                       # Create .env and the database password files in .secrets/
make dev                            # Start dev stack (compose watch) on http://localhost:3000
make up                             # Start production stack (detached) on http://localhost
make down                           # Stop stack
make down-v                         # Stop stack and remove volumes
make build                          # Build Docker images
make status                         # Show running containers
make logs                           # Tail all logs
make logs-backend                   # Tail backend logs
make logs-frontend                  # Tail frontend logs
make restart                        # Restart all services
make migration                      # Apply migrations (rebuilds the backend image, runs migrate)
make generate-migration MSG=<name>  # Generate a migration from the modules' schema.sql
make seed ADMIN_ACCOUNT=<name>      # Create the admin user (password read from the terminal)
make reset-db ADMIN_ACCOUNT=<name>  # Delete all local data, restart, seed
make rename NAME=<name>             # Rename a fresh clone (optional MODULE=<go module path>)

# Backend development (from backend/)
make run              # Start the API server with backend/.env
make test             # Integration tests
make check            # Everything CI runs
make gen              # Regenerate the HTTP server from api/openapi.yaml
make sqlc             # Regenerate query code

# Frontend development (from frontend/)
pnpm dev              # Start Next.js dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript check + Biome lint
pnpm format           # Biome format
pnpm gen:api          # Regenerate API types from backend/api/openapi.yaml
```

## Architecture

### Backend and frontend

- Backend (Go modular monolith, oapi-codegen strict server, pgx + sqlc, Atlas, opaque sessions): `backend/AGENTS.md`.
- Frontend (Next.js App Router, feature-based, next-intl, generated API client): `frontend/AGENTS.md`.
- Decisions: `docs/adr/`.

### Docker Compose Deployment

Two compose files for different environments:
- **`docker-compose.yml`** — production mode (built images); only Caddy publishes ports (80/443)
- **`docker-compose.dev.yml`** — dev overrides (`docker compose watch` rebuilds the backend, frontend dev server); Caddy on `127.0.0.1:3000`

Services: postgres → migrate (`app migrate`, Atlas) → backend (:8000, internal) → frontend (:3000, internal) → caddy (`/api/*` to the backend, the rest to Next). Deployment guide: `docs/deploy-vps.md`.

## Design Rules

開發新功能前，必須先閱讀對應的設計規則。這些規則是強制性的，所有程式碼必須遵守：

- **Database 設計**：`docs/rules/database-design.md` — 命名規範、schema 與型別、查詢、Migration
- **API 設計**：`docs/rules/api-design.md` — URL 設計、回應與錯誤格式、分頁、契約與認證規範

## Conventions

- Backend is Go (`go.mod` sets the version), queries only through sqlc, schema only through Atlas; frontend uses `pnpm`.
