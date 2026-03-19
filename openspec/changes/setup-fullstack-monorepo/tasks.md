## 1. Backend Migration

- [x] 1.1 Copy all files from `fastapi-template` into `backend/` (src/, tests/, alembic/, alembic.ini, pyproject.toml, Makefile, Dockerfile, CLAUDE.md, uv.lock, requirements/)
- [x] 1.2 Remove `docker-compose.yml-example` and `ansible/` from `backend/` (superseded by k8s manifests; ansible is deployment-specific)
- [x] 1.3 Verify `backend/Dockerfile` builds correctly standalone (`docker build ./backend`)
- [x] 1.4 Verify `backend/alembic.ini` paths are correct (script_location = alembic is already relative)
- [x] 1.5 Verify `backend/Makefile` commands work when run from `backend/` directory

## 2. Next.js Frontend Setup

- [x] 2.1 Rename `portfolio/` to `frontend/` and clean business content (keep architecture, remove portfolio-specific pages/data/components)
- [x] 2.2 Create `frontend/Dockerfile` for production build (multi-stage: install deps → build → serve with standalone output)
- [x] 2.3 Add `NEXT_PUBLIC_API_URL` environment variable support for backend API configuration (already in api-client.ts)
- [x] 2.4 Verify `pnpm dev` starts the dev server successfully from `frontend/`

## 3. k3s Kubernetes Manifests

- [x] 3.1 Create `k8s/namespace.yaml` with dedicated project namespace
- [x] 3.2 Create PostgreSQL manifests: `k8s/postgres/secret.yaml` (template), `pvc.yaml`, `deployment.yaml`, `service.yaml`
- [x] 3.3 Create backend manifests: `k8s/backend/secret.yaml` (DATABASE_URL, SECRET_KEY), `deployment.yaml` (with init container for alembic migrations), `service.yaml`
- [x] 3.4 Create frontend manifests: `k8s/frontend/configmap.yaml` (NEXT_PUBLIC_API_URL), `deployment.yaml`, `service.yaml` (NodePort)
- [x] 3.5 Create `.env-example` at monorepo root with all required environment variables

## 4. Root Makefile

- [x] 4.1 Create root `Makefile` with targets: `build` (build images + import to k3s), `deploy` (apply manifests), `teardown` (delete namespace), `logs` (tail pod logs), `status` (show pods/services)
- [x] 4.2 Add `.env` to `.gitignore`

## 5. Claude Code Configuration

- [x] 5.1 Create root `CLAUDE.md` with monorepo structure overview, backend conventions summary (referencing `backend/CLAUDE.md`), frontend conventions, and k3s operations
- [x] 5.2 Set up `.claude/skills/` at monorepo root with backend skills adapted for monorepo context

## 6. Verification

- [x] 6.1 Verify `make build` completes successfully for all images
- [x] 6.2 Verify `make deploy` starts all three services with correct dependency order on k3d
