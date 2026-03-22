## Why

專案經過全面 code review，發現多個影響生產環境穩定性、安全性和程式碼品質的問題。K8s 部署缺少 resource limits 和 securityContext 可能導致記憶體耗盡；前端 auth token 處理有安全漏洞；後端缺少必要的 DB index 和驗證。這些問題需要在進入下一階段功能開發前修復。

## What Changes

### Batch 1: Critical
- K8s 所有 deployment 加入 resource requests/limits 和 securityContext（runAsNonRoot, drop ALL capabilities）
- Frontend deployment 加入 liveness probe
- 所有 K8s probe 標準化，加入 timeoutSeconds/failureThreshold
- Backend `items.owner_id` 加入 DB index 並產生 migration
- Backend Dockerfile CMD 移除重複的 migration 指令（保留 K8s init container）
- Frontend 移除 `lib/validations.ts` 中重複且不一致的 `loginSchema`

### Batch 2: Important
- Frontend `api-client.ts` 加入 401 攔截、自動 refresh token、retry 機制
- Backend `RefreshTokenRepository` 改為繼承 `BaseRepository[RefreshToken]`
- Backend `UserCreateInput.account` 加入 min/max length 驗證
- Backend 移除未使用的 `ListDataResponse` schema
- Root `Makefile` 的 `deploy` target 加入 `build` 依賴
- Backend Dockerfile 改為 multi-stage build，分離 build dependencies（gcc）

### Batch 3: Minor
- Frontend 移除未使用的 `zustand` dependency
- Dashboard sidebar 移除不存在的頁面連結（products, customers, settings）
- Dashboard sidebar labels 改用 i18n keys
- K8s PVC 加入 `storageClassName: local-path`

## Capabilities

### New Capabilities
- `k8s-hardening`: K8s deployment 安全強化 — resource limits, securityContext, probe 標準化, PVC storageClassName
- `auth-token-refresh`: Frontend 401 自動 refresh token 和 retry 機制
- `backend-cleanup`: Backend 程式碼品質修復 — index, validation, repository 繼承, 移除 dead code
- `frontend-cleanup`: Frontend 程式碼品質修復 — 移除重複 schema, 未使用 dependency, sidebar 修正
- `dockerfile-optimization`: Dockerfile multi-stage build 和移除重複 migration

### Modified Capabilities

（無既有 spec 需要修改）

## Impact

- **K8s manifests**: `k8s/backend/deployment.yaml`, `k8s/frontend/deployment.yaml`, `k8s/postgres/deployment.yaml`, `k8s/postgres/pvc.yaml`
- **Backend code**: `src/items/models.py`, `src/auth/repository.py`, `src/auth/schemas.py`, `src/core/schemas/base.py`
- **Backend infra**: `backend/Dockerfile`
- **Frontend code**: `src/lib/api-client.ts`, `src/lib/validations.ts`, `src/features/auth/hooks/use-auth.ts`
- **Frontend deps**: `package.json`（移除 zustand）
- **Frontend UI**: `src/app/[locale]/(dashboard)/layout.tsx`（sidebar）, i18n messages
- **Root**: `Makefile`
- **Alembic**: 新增 migration（owner_id index）
