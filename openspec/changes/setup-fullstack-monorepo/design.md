## Context

We have a production-ready FastAPI backend in `/Users/will/github/fastapi-template` using Pragmatic DDD architecture with async SQLAlchemy, JWT auth, and modular CRUD patterns. It currently runs standalone with its own Dockerfile and docker-compose. The goal is to create a fullstack monorepo template that pairs this backend with a Next.js frontend, orchestrated entirely via k3s (lightweight Kubernetes), for use in client projects.

Current backend stack: Python 3.12, FastAPI, SQLAlchemy 2.0 (async/asyncpg), JWT (python-jose), bcrypt (passlib), Alembic, structlog, Ruff, uv, pytest-asyncio.

## Goals / Non-Goals

**Goals:**
- Move the FastAPI backend into `backend/` with minimal changes — preserve all existing architecture, tests, and migrations
- Create a basic Next.js frontend in `frontend/` scaffolded and ready for feature development
- Provide k3s Kubernetes manifests in `k8s/` that bring up PostgreSQL, backend, and frontend together
- Provide a root Makefile with common k3s operations (deploy, teardown, logs, build images)
- Create a root `CLAUDE.md` that covers conventions for both stacks
- Keep the template production-ready: images build, services communicate on k3s, dev workflow is smooth

**Non-Goals:**
- Building out frontend features (pages, components, API integration) — this is scaffolding only
- Changing the backend architecture or adding new backend modules
- CI/CD pipeline setup (GitHub Actions, GitLab CI)
- Authentication integration between frontend and backend (future work)
- Helm charts or Kustomize — plain manifests are sufficient for this template
- Ingress controller or TLS setup (can be added per-project)

## Decisions

### 1. Monorepo structure: `backend/` + `frontend/` + `k8s/` at root

Each stack lives in its own top-level directory with independent tooling (uv for Python, npm/pnpm for Node). Kubernetes manifests live in `k8s/`. No workspace manager (nx, turborepo).

**Why over monorepo tools**: The two stacks share nothing at build time. A workspace manager adds complexity with no benefit for a two-service template.

### 2. Backend migration: copy files, adjust paths

Copy the entire fastapi-template contents into `backend/`. Adjustments needed:
- `Dockerfile`: `COPY . /app` stays the same; k8s Deployment references the built image
- `Makefile`: paths stay relative to `backend/`, no changes needed since `make` runs from `backend/`
- `alembic.ini`: `script_location = alembic` is already relative, no change needed
- Remove `docker-compose.yml-example` from backend (replaced by k8s manifests)

**Why copy instead of git subtree/submodule**: Clean break. The template should be self-contained. No ongoing sync with the original repo.

### 3. Next.js frontend: App Router with TypeScript

Use `create-next-app` with App Router (not Pages Router), TypeScript, and Tailwind CSS. This is the current Next.js standard.

**Why App Router**: It's the default and recommended approach for new Next.js projects. Pages Router is legacy.

### 4. k3s orchestration: plain Kubernetes manifests

All services run on k3s using plain YAML manifests in `k8s/`:

```
k8s/
├── namespace.yaml          # project namespace
├── postgres/
│   ├── pvc.yaml            # PersistentVolumeClaim for data
│   ├── secret.yaml         # DB credentials (template)
│   ├── deployment.yaml     # PostgreSQL Deployment
│   └── service.yaml        # ClusterIP Service
├── backend/
│   ├── secret.yaml         # SECRET_KEY, DATABASE_URL
│   ├── deployment.yaml     # FastAPI Deployment (init container for migrations)
│   └── service.yaml        # ClusterIP or NodePort Service
└── frontend/
    ├── configmap.yaml      # NEXT_PUBLIC_API_URL
    ├── deployment.yaml     # Next.js Deployment
    └── service.yaml        # NodePort Service (port 3000→30000)
```

Key design choices:
- **Init container for migrations**: Backend Deployment uses an init container running `alembic upgrade head` before the main FastAPI container starts
- **Secrets as templates**: `secret.yaml` files use placeholder values; actual values supplied via `.env` + envsubst or manual editing
- **NodePort for frontend**: Exposes frontend on a fixed port for local access without ingress. Backend accessible via ClusterIP internally or NodePort for debugging.
- **Namespace isolation**: All resources in a dedicated namespace for clean teardown (`kubectl delete namespace`)

**Why k3s over Docker Compose**: Consistent dev-to-prod environment. Same manifests work locally and on production servers. k3s is lightweight enough for single-machine dev.

**Why plain manifests over Helm/Kustomize**: Simplicity. A template with 3 services doesn't need chart templating. Easy to read, easy to customize per-project.

### 5. Root Makefile for k3s operations

A root `Makefile` provides common operations:
- `make build` — build Docker images for backend and frontend
- `make deploy` — apply all k8s manifests
- `make teardown` — delete the namespace
- `make logs` — tail logs from all pods
- `make status` — show pod/service status

**Why Makefile**: Familiar, zero dependencies, works everywhere. Developers don't need to remember kubectl commands.

### 6. CLAUDE.md: root-level with backend/frontend sections

A single root `CLAUDE.md` with sections for:
- Monorepo conventions (directory structure, how to run things)
- Backend conventions (referencing the existing patterns)
- Frontend conventions (to be filled as frontend matures)
- k3s operations (how to deploy, check logs, teardown)

The existing backend `CLAUDE.md` moves into `backend/CLAUDE.md` for backend-specific context.

## Risks / Trade-offs

- **[Backend path breakage]** → Mitigated by keeping all backend paths relative; Dockerfile and alembic.ini already use relative paths. Run tests after migration to verify.
- **[Frontend is minimal]** → Acceptable trade-off. The goal is scaffolding, not a complete frontend. Features come later through additional OpenSpec changes.
- **[k3s required on dev machines]** → Developers must install k3s locally. Mitigated by clear setup instructions in README and CLAUDE.md. k3s install is a single command.
- **[No shared types between frontend/backend]** → Non-goal for now. Can add OpenAPI codegen later.
- **[Secret management is basic]** → Template uses placeholder secrets in YAML. Production deployments should use sealed-secrets or external secret managers. Documented as a TODO.
