# Fullstack Template

Production-ready fullstack monorepo for new web projects.

**Backend:** Go (modular monolith, OpenAPI contract-first, pgx + sqlc, Atlas migrations, opaque session cookies)
**Frontend:** Next.js 16 (App Router, TypeScript, Tailwind v4, shadcn/ui, i18n, generated API client)
**Infra:** Docker Compose behind Caddy, frontend and API on one origin (`/api`)

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (Compose v2.24.7+ for `make dev`, which runs `docker compose up --watch`)
- [Go](https://go.dev/dl/) — version in `backend/go.mod`; for backend development, tests and `make rename`
- [pnpm](https://pnpm.io/) — `npm install -g pnpm`
- Python 3 — for the repo scripts (`make rename`, `make init-env`)

## Quick Start

```bash
# 1. Clone and rename for your project
git clone <this-repo> my-project
cd my-project
make rename NAME=my-project          # optional: MODULE=example.com/me/my-project/backend

# 2. Setup environment: .env from .env-example, database passwords in .secrets/
make init-env

# 3. Start dev stack (rebuilds the backend on change, Next.js dev server)
make dev

# App:      http://localhost:3000
# API:      http://localhost:3000/api
# API docs: http://localhost:3000/api/docs (MODE=dev)

# 4. Create an admin account (in another terminal)
make seed ADMIN_ACCOUNT=admin
```

### On GitHub, once per new repository

Repository settings do not come with a copy of the template. Under Settings → Code security, turn on Dependabot alerts,
Dependabot security updates (a fix pull request for each new advisory, without the 7-day cooldown that version updates
wait), secret scanning and push protection. The checks in `.github/workflows/` run without further setup.

## Project Structure

```
├── backend/                Go API (contract in backend/api/openapi.yaml)
├── frontend/               Next.js 16 (React 19, TypeScript, Tailwind v4)
├── deploy/                 Caddyfile, PostgreSQL initdb scripts
├── docker-compose.yml      Production stack
├── docker-compose.dev.yml  Dev overrides
├── Makefile                Docker Compose shortcuts
├── scripts/                rename and init_env
├── .env-example            Environment variable template
└── docs/                   Design rules, decisions (adr/), deployment guide
```

## Development

### Docker Compose (from root)

```bash
make dev                            # Start dev stack (http://localhost:3000)
make up                             # Start production stack (detached, http://localhost)
make down                           # Stop stack
make down-v                         # Stop stack + remove volumes
make build                          # Build Docker images
make status                         # Show running containers
make logs                           # Tail all logs
make logs-backend                   # Tail backend logs only
make migration                      # Apply migrations
make generate-migration MSG=<name>  # Generate a migration from the modules' schema.sql
make seed ADMIN_ACCOUNT=<name>      # Create the admin user
make reset-db ADMIN_ACCOUNT=<name>  # Reset DB (clear volumes + migrate + seed)
```

### Backend (from `backend/`)

```bash
make run                  # API server (port 8000) with backend/.env
make test                 # Integration tests (Docker)
make check                # Everything CI runs
make gen                  # Regenerate the HTTP server from the contract
make sqlc                 # Regenerate query code
```

### Frontend (from `frontend/`)

```bash
pnpm dev                  # Next.js dev server (port 3000, Turbopack)
pnpm test                 # Vitest
pnpm lint                 # TypeScript + Biome
pnpm build                # Production build
pnpm gen:api              # Regenerate API types from the contract
```

## Starting a New Project

```bash
make rename NAME=my-client-project
```

This renames Docker images, package names, database and role names, branding, and the Go module path
(default `github.com/Will413028/<NAME>/backend`, override with `MODULE=`), then checks that the backend builds.

## Architecture

See `AGENTS.md` for the overview, or:

- `backend/AGENTS.md` — Backend layout, layering, errors, adding a module
- `frontend/AGENTS.md` — Frontend feature modules, auth flow, conventions
- `docs/rules/` — API and database design rules
- `docs/adr/` — Decisions and their reasons

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go, stdlib `net/http`, oapi-codegen (strict server) + kin-openapi validator, pgx, sqlc, `log/slog` |
| Database | PostgreSQL 18, Atlas declarative schema (migrations generated from `schema.sql`), UUIDv7 keys |
| API | OpenAPI 3.0.3 contract-first, RFC 9457 problems, keyset pagination, JSON Merge Patch |
| Auth | Opaque DB sessions in a `__Host-` HttpOnly cookie, `CrossOriginProtection`, argon2id, per-account throttling, breached-password list |
| Frontend | Next.js 16, React 19, TypeScript, Tailwind CSS v4 |
| UI | shadcn/ui, Lucide icons |
| State | Tanstack React Query |
| Forms | React Hook Form + Zod |
| API client | openapi-typescript + openapi-fetch |
| i18n | next-intl (English, Traditional Chinese) |
| Linting | golangci-lint (backend), Biome (frontend) |
| Testing | Go integration tests with testcontainers (backend), Vitest + RTL (frontend) |
| Infra | Docker Compose, Caddy (automatic HTTPS) |
