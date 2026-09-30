# Fullstack Template

Production-ready fullstack monorepo with FastAPI backend, Next.js frontend, and Docker Compose deployment.

## Verification

- Backend tests use `pytest`, frontend tests use `vitest`; a task is complete only when `make test` (backend) and `pnpm test` (frontend) pass, as CI runs them.
- Cover observable behavior, failure conditions, and regressions; do not add tests per module or function when existing tests already cover the behavior. Docs-only or formatting changes need only the relevant static checks.
- Coverage target: backend 80%+ (enforced in CI); frontend covers critical paths.

## Constraints

- Never edit an Alembic migration that has already been applied; add a new one with `make generate-migration`.
- `make down-v` deletes the Postgres volume and all local data.
- Never commit `.env`; add new variables to `.env-example`.

## Monorepo Structure

```
├── backend/                # FastAPI (Pragmatic DDD, async SQLAlchemy)
├── frontend/               # Next.js 16 (App Router, TypeScript, Tailwind, shadcn/ui)
├── docker-compose.yml      # Production stack (postgres + backend + frontend)
├── docker-compose.dev.yml  # Dev overrides (hot reload, volume mounts)
├── Makefile                # Docker Compose shortcuts
└── .env-example            # Environment variable template
```

## Quick Reference

```bash
# Docker Compose (from root)
make dev              # Start dev stack with hot reload
make up               # Start production stack (detached)
make down             # Stop stack
make down-v           # Stop stack and remove volumes
make build            # Build Docker images
make status           # Show running containers
make logs             # Tail all logs
make logs-backend     # Tail backend logs
make logs-frontend    # Tail frontend logs
make restart          # Restart all services
make migration        # Run Alembic migrations
make seed             # Create admin user

# Backend development (from backend/)
make run              # Start FastAPI dev server
make test             # Run pytest
make lint             # Ruff format + check

# Frontend development (from frontend/)
pnpm dev              # Start Next.js dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript check + Biome lint
pnpm format           # Biome format
```

## Architecture

### Backend and frontend

- Backend (FastAPI, Pragmatic DDD, async SQLAlchemy): `backend/AGENTS.md`.
- Frontend (Next.js App Router, feature-based, next-intl): `frontend/AGENTS.md`.

### Docker Compose Deployment

Two compose files for different environments:
- **`docker-compose.yml`** — production mode (built images)
- **`docker-compose.dev.yml`** — dev overrides (volume mounts, hot reload)

Services: postgres → migrate (alembic) → backend (:8000) → frontend (:3000)

## Design Rules

開發新功能前，必須先閱讀對應的設計規則。這些規則是強制性的，所有程式碼必須遵守：

- **Database 設計**：`docs/rules/database-design.md` — 命名規範、Model 結構、型別選擇、Migration
- **API 設計**：`docs/rules/api-design.md` — URL 設計、Response 格式、Schema/Router/Service 規範

## Conventions

- Backend uses async SQLAlchemy only, managed with `uv`; frontend uses `pnpm`.
