## Why

We have a production-ready FastAPI backend (Pragmatic DDD architecture) in a separate repo (`fastapi-template`). To create a reusable fullstack template for client projects, we need to consolidate it into a monorepo alongside a Next.js frontend, orchestrated via k3s (lightweight Kubernetes). This enables spinning up new projects from a single, cohesive template with a consistent dev-to-prod workflow.

## What Changes

- Move the existing FastAPI backend into `backend/` subdirectory, adjusting paths in Makefile, Dockerfile, and alembic.ini
- Create a Next.js frontend in `frontend/` with basic project structure
- Add k3s Kubernetes manifests in `k8s/` for orchestrating PostgreSQL, backend, and frontend services
- Provide a Makefile or scripts at root for common k3s operations (deploy, teardown, logs)
- Create a root `CLAUDE.md` covering conventions for both frontend and backend
- Set up monorepo-level `.claude/skills/` for Claude Code integration

## Capabilities

### New Capabilities
- `monorepo-structure`: Monorepo layout with backend/ and frontend/ directories, k8s/ for Kubernetes manifests, and root-level configuration
- `backend-migration`: Migration of FastAPI backend from standalone repo into backend/ subdirectory with all path adjustments
- `nextjs-frontend`: Next.js frontend scaffolding with basic structure ready for feature development
- `k3s-orchestration`: k3s Kubernetes manifests and tooling for PostgreSQL, FastAPI backend, and Next.js frontend services (dev and prod)
- `claude-code-config`: Root CLAUDE.md and .claude/skills/ covering fullstack conventions

### Modified Capabilities
<!-- No existing capabilities to modify -->

## Impact

- **Code**: Entire FastAPI codebase moves into `backend/`; new `frontend/` directory created from scratch
- **Dependencies**: Adds Node.js/npm ecosystem alongside existing Python/uv stack
- **Infrastructure**: k3s Kubernetes manifests in `k8s/` replace Docker Compose; backend Dockerfile WORKDIR and build context change; k3s required on dev machines
- **Developer workflow**: All development happens from monorepo root; services managed via k3s; `CLAUDE.md` and skills apply to both stacks
