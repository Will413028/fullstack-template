# Fullstack Template

Production-ready fullstack monorepo for client projects.

**Backend:** FastAPI (Pragmatic DDD, async SQLAlchemy, JWT + refresh tokens)
**Frontend:** Next.js 16 (App Router, TypeScript, Tailwind v4, shadcn/ui, i18n)
**Infra:** k3s (Kubernetes)

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [k3d](https://k3d.io/) — `brew install k3d`
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — `brew install kubectl`
- [uv](https://docs.astral.sh/uv/) — `curl -LsSf https://astral.sh/uv/install.sh | sh`
- [pnpm](https://pnpm.io/) — `npm install -g pnpm`
- [envsubst](https://www.gnu.org/software/gettext/) — `brew install gettext`

## Quick Start

```bash
# 1. Clone and rename for your project
git clone <this-repo> my-project
cd my-project
make rename NAME=my-project

# 2. Setup environment
cp .env-example .env
# Edit .env with your values (SECRET_KEY must be 32+ chars)

# 3. Create k3d cluster
make cluster-up

# 4. Build and deploy
make build
make deploy

# 5. Verify
make status
# Frontend: http://localhost:30000
# Backend:  http://localhost:30800
# API docs: http://localhost:30800/docs (dev mode)
```

## Project Structure

```
├── backend/          FastAPI (Python 3.12, async SQLAlchemy, JWT)
├── frontend/         Next.js 16 (React 19, TypeScript, Tailwind v4)
├── k8s/              Kubernetes manifests (plain YAML)
├── Makefile          k3d cluster + build + deploy commands
├── .env-example      Environment variable template
└── docs/             Architecture decisions and known issues
```

## Development

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

### k3d / Kubernetes (from root)

```bash
make cluster-up           # Create k3d cluster
make build                # Build images + import to k3d
make deploy               # Apply all manifests
make status               # Show pods and services
make logs                 # Tail all logs
make logs-backend         # Tail backend logs only
make teardown             # Delete namespace
make cluster-down         # Delete cluster
```

## Starting a New Project

```bash
make rename NAME=my-client-project
```

This renames the k3d cluster, k8s namespace, Docker images, and package names throughout the project.

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
| State | Tanstack React Query (server), Zustand (client) |
| Forms | React Hook Form + Zod validation |
| i18n | next-intl (English, Traditional Chinese) |
| Linting | Ruff (backend), Biome (frontend) |
| Testing | pytest + pytest-asyncio (backend), Vitest + RTL (frontend) |
| Infra | k3d/k3s, plain Kubernetes YAML manifests |
