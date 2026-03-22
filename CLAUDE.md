# Fullstack Template

Production-ready fullstack monorepo with FastAPI backend, Next.js frontend, and k3d/k3s deployment.

## Monorepo Structure

```
├── backend/          # FastAPI (Pragmatic DDD, async SQLAlchemy)
├── frontend/         # Next.js 16 (App Router, TypeScript, Tailwind, shadcn/ui)
├── k8s/              # Kubernetes manifests (plain YAML)
├── Makefile          # Root-level k3d/k3s operations
└── .env-example      # Environment variable template
```

## Quick Reference

```bash
# k3d cluster (from root)
make cluster-up       # Create k3d cluster (first time)
make cluster-down     # Delete k3d cluster

# Build & deploy (from root)
make build            # Build Docker images + import into k3d
make deploy           # Apply all k8s manifests
make teardown         # Delete namespace and all resources
make status           # Show cluster, pods, and services
make logs             # Tail all pod logs
make logs-backend     # Tail backend logs
make logs-frontend    # Tail frontend logs
make restart          # Rolling restart all deployments

# Backend development (from backend/)
make run              # Start FastAPI dev server
make test             # Run pytest
make lint             # Ruff format + check
make migration        # Apply Alembic migrations
make generate_migration  # Auto-generate migration

# Frontend development (from frontend/)
pnpm dev              # Start Next.js dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript check + Biome lint
pnpm format           # Biome format
```

## Architecture

### Backend — Pragmatic DDD

See `backend/CLAUDE.md` for full details. Key patterns:
- **Service layer NEVER raises HTTPException** — only `AppException` subclasses
- **Router has NO try/except** — global exception handler catches all
- **BaseRepository[T]** — generic async CRUD
- **All DB operations are async** (AsyncSession + asyncpg)

### Frontend — Feature-based Next.js

- **Next.js 16** with App Router, React 19, TypeScript
- **pnpm** package manager
- **Tailwind CSS v4** + **shadcn/ui** components
- **Biome** for linting/formatting (not ESLint)
- **Tanstack Query** for server state, **Zustand** for client state
- **next-intl** for i18n (en, zh-TW)
- **React Hook Form** + **Zod** for form validation

Frontend structure:
```
src/
├── app/[locale]/         # File-based routing with i18n
│   ├── (auth)/           # Auth pages (login)
│   ├── (dashboard)/      # Protected pages
│   └── (marketing)/      # Public pages
├── components/
│   ├── ui/               # shadcn/ui components
│   ├── shared/           # Reusable components
│   └── layout/           # Header, footer
├── features/             # Feature modules (auth, etc.)
├── hooks/                # Custom React hooks
├── lib/                  # Utilities (api-client, animations, validations)
├── store/                # Zustand stores
├── providers/            # React Query provider
├── i18n/                 # Internationalization config
└── types/                # Global type definitions
```

### k3d/k3s Deployment

Uses k3d (k3s in Docker) for consistent dev/prod environments. Same commands on macOS and Linux.

Plain Kubernetes manifests in `k8s/`:
- **Namespace isolation** — all resources in `fullstack` namespace
- **Secrets** — templated with `envsubst` from `.env` file
- **Init container** — backend runs Alembic migrations before starting
- **NodePort** — frontend on 30000, backend on 30800
- **k3d image import** — local images imported directly, no registry needed

## Design Rules

開發新功能前，必須先閱讀對應的設計規則。這些規則是強制性的，所有程式碼必須遵守：

- **Database 設計**：`docs/rules/database-design.md` — 命名規範、Model 結構、型別選擇、Migration
- **API 設計**：`docs/rules/api-design.md` — URL 設計、Response 格式、Schema/Router/Service 規範
- **Redis 設計**：`docs/rules/redis-design.md` — Key 命名、TTL 策略、Cache Pattern

## Conventions

- Communicate in 繁體中文
- Use async SQLAlchemy (never sync) on backend
- Use Biome (not ESLint/Prettier) on frontend
- Use pnpm (not npm/yarn) on frontend
- Use uv (not pip) on backend
