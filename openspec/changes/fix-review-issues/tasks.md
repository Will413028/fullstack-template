## 1. K8s Hardening

- [x] 1.1 Add resource requests/limits to backend deployment (main container + init container)
- [x] 1.2 Add resource requests/limits to frontend deployment
- [x] 1.3 Add resource requests/limits to postgres deployment
- [x] 1.4 Add securityContext to backend deployment (pod + containers)
- [x] 1.5 Add securityContext to frontend deployment (pod + container)
- [x] 1.6 Add securityContext to postgres deployment (pod + container)
- [x] 1.7 Add liveness probe to frontend deployment
- [x] 1.8 Standardize all probes with timeoutSeconds and failureThreshold (backend, frontend, postgres)
- [x] 1.9 Add storageClassName: local-path to postgres PVC

## 2. Backend Cleanup

- [x] 2.1 Add index to items.owner_id in models.py and generate Alembic migration
- [x] 2.2 Refactor RefreshTokenRepository to extend BaseRepository[RefreshToken]
- [x] 2.3 Add min_length/max_length validation to UserCreateInput.account
- [x] 2.4 Remove unused ListDataResponse from core/schemas/base.py
- [x] 2.5 Run ruff check + pytest to verify no regressions

## 3. Dockerfile & Makefile Optimization

- [x] 3.1 Convert backend Dockerfile to multi-stage build (builder + runtime)
- [x] 3.2 Remove alembic migration from Dockerfile CMD (keep only fastapi run)
- [x] 3.3 Add build dependency to Makefile deploy target

## 4. Frontend Cleanup

- [x] 4.1 Remove duplicate loginSchema from lib/validations.ts
- [x] 4.2 Remove zustand from package.json dependencies and clean up store directory
- [x] 4.3 Remove non-existent page links from dashboard sidebar (products, customers, settings)
- [x] 4.4 Convert dashboard sidebar labels to use i18n (useTranslations)
- [x] 4.5 Add i18n keys for sidebar labels to en.json and zh-TW.json message files (already existed)

## 5. Auth Token Refresh

- [x] 5.1 Implement token refresh logic in api-client.ts (intercept 401, refresh, retry)
- [x] 5.2 Add mutex/flag to prevent concurrent refresh requests
- [x] 5.3 Add redirect to login on refresh failure
- [x] 5.4 Add unit tests for 401 refresh flow in api-client.test.ts

## 6. Verification

- [x] 6.1 Run backend lint (ruff) — passed
- [x] 6.2 Run frontend lint (biome) — passed
- [x] 6.3 Run frontend build — passed
- [ ] 6.4 Verify K8s manifests with kubectl dry-run (skipped — no cluster available)
