# Fullstack Template

Production-ready fullstack monorepo for client projects.

**Backend:** FastAPI (Pragmatic DDD, async SQLAlchemy, JWT + refresh tokens)
**Frontend:** Next.js 16 (App Router, TypeScript, Tailwind v4, shadcn/ui, i18n)
**Infra:** Docker Compose

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (with Compose v2)
- [uv](https://docs.astral.sh/uv/) — `curl -LsSf https://astral.sh/uv/install.sh | sh`
- [pnpm](https://pnpm.io/) — `npm install -g pnpm`

## Quick Start

```bash
# 1. Clone and rename for your project
git clone <this-repo> my-project
cd my-project
make rename NAME=my-project

# 2. Setup environment
make init-env
# Edit .env to adjust CORS_ORIGINS or other configurations if needed

# 3. Start dev stack (hot reload)
make dev

# Frontend: http://localhost:3000
# Backend:  http://localhost:8000
# API docs: http://localhost:8000/docs
```

## Project Structure

```
├── backend/                FastAPI (Python 3.12, async SQLAlchemy, JWT)
├── frontend/               Next.js 16 (React 19, TypeScript, Tailwind v4)
├── docker-compose.yml      Production stack
├── docker-compose.dev.yml  Dev overrides (hot reload)
├── Makefile                Docker Compose shortcuts
├── .env-example            Environment variable template
└── docs/                   Design rules and architecture notes
```

## Development

### Docker Compose (from root)

```bash
make dev                  # Start dev stack (hot reload)
make up                   # Start production stack (detached)
make down                 # Stop stack
make down-v               # Stop stack + remove volumes
make build                # Build Docker images
make status               # Show running containers
make logs                 # Tail all logs
make logs-backend         # Tail backend logs only
make migration            # Run Alembic migrations
make seed                 # Create admin user
```

### Backend (from `backend/`)

```bash
make run                  # FastAPI dev server (port 8000)
make test                 # pytest
make lint                 # Ruff format + check
make generate_migration   # Alembic auto-generate
make migration            # Apply migrations
```

### Frontend (from `frontend/`)

```bash
pnpm dev                  # Next.js dev server (port 3000, Turbopack)
pnpm test                 # Vitest
pnpm lint                 # TypeScript + Biome
pnpm build                # Production build
```

## Starting a New Project

```bash
make rename NAME=my-client-project
```

This renames Docker images, package names, and branding throughout the project.

## Architecture

See `CLAUDE.md` for detailed architecture documentation, or:

- `backend/CLAUDE.md` — Backend DDD patterns, error handling, module structure
- `frontend/CLAUDE.md` — Frontend feature modules, auth flow, conventions

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Python 3.12, FastAPI, SQLAlchemy 2.0 (async), asyncpg, Alembic |
| Auth | JWT (PyJWT) + refresh tokens, bcrypt, rate limiting (slowapi) |
| Frontend | Next.js 16, React 19, TypeScript, Tailwind CSS v4 |
| UI | shadcn/ui, Framer Motion, Lucide icons |
| State | Tanstack React Query |
| Forms | React Hook Form + Zod validation |
| i18n | next-intl (English, Traditional Chinese) |
| Linting | Ruff (backend), Biome (frontend) |
| Testing | pytest + pytest-asyncio (backend), Vitest + RTL (frontend) |
| Infra | Docker Compose |
