## Context

專案經全面 review 後發現 16 個問題，涵蓋 K8s 部署安全、前端 auth 機制、後端程式碼品質、Docker 優化。目前所有 K8s deployment 無 resource limits 和 securityContext，生產環境有記憶體耗盡和提權風險。前端 auth token 過期後無自動 refresh，使用者體驗差。後端有缺失 index、重複程式碼、未使用的 schema 等技術債。

## Goals / Non-Goals

**Goals:**
- K8s deployment 達到生產級安全標準（resource limits, securityContext, 完整 probes）
- 前端 auth 401 自動 refresh + retry，消除 token 過期造成的中斷
- 後端消除效能瓶頸（owner_id index）和技術債（repository 繼承, validation, dead code）
- Dockerfile 優化減少 image size，消除重複 migration
- 前端清理未使用 dependency 和不一致的 schema

**Non-Goals:**
- 不重構整體架構（保持 Pragmatic DDD）
- 不新增功能（如 dashboard 的 products/customers/settings 頁面）
- 不修改 cookie 為 HttpOnly（需後端配合設定 Set-Cookie，列為後續獨立 change）
- 不導入 NetworkPolicy（列為後續 K8s hardening phase 2）

## Decisions

### 1. K8s resource limits 數值

| Container | CPU request | CPU limit | Memory request | Memory limit |
|-----------|------------|-----------|----------------|--------------|
| backend | 100m | 500m | 128Mi | 512Mi |
| frontend | 100m | 300m | 128Mi | 384Mi |
| postgres | 200m | 1000m | 256Mi | 1Gi |
| backend init (migrate) | 100m | 300m | 128Mi | 256Mi |

**理由**: 基於 k3d 單節點環境的合理預設。生產環境應根據實際 profiling 調整。

### 2. securityContext 策略

所有 pod 統一設定：
- `runAsNonRoot: true`
- `allowPrivilegeEscalation: false`
- `capabilities.drop: [ALL]`
- Backend/frontend: `runAsUser: 1001`（對應 Dockerfile 的 appuser/nextjs）
- Postgres: `runAsUser: 999`（官方 postgres image 的 postgres user）、不設 `readOnlyRootFilesystem`（postgres 需要寫入 data dir）

**理由**: 最小權限原則。Dockerfile 已建立非 root user，K8s 層強制執行。

### 3. Frontend 401 refresh 策略

在 `api-client.ts` 的 `request()` 方法中攔截 401：
1. 收到 401 → 用 refresh_token 呼叫 `/auth/refresh`
2. 成功 → 更新 cookie，用新 token retry 原始 request
3. 失敗 → 清除 cookies，redirect 到 login
4. 用 flag 防止 refresh 過程中的並發 request 重複 refresh（mutex pattern）

**替代方案**: Axios interceptor — 但專案用自製 api-client，保持一致不引入新 dependency。

### 4. RefreshTokenRepository 繼承策略

改為 `class RefreshTokenRepository(BaseRepository[RefreshToken])`，但保留 `get_by_token_hash()` 和 `revoke_all_for_user()` 等自定義方法。`create()` 可直接使用 BaseRepository 的實作。

**理由**: 消除重複 CRUD 程式碼，保持架構一致性。

### 5. Dockerfile multi-stage build

```
Stage 1 (builder): python:3.12-slim + gcc + libpq-dev → uv sync
Stage 2 (runtime): python:3.12-slim + libpq-dev only → COPY --from=builder
```

CMD 只啟動 FastAPI，不跑 migration（由 K8s init container 負責）。本地開發仍可用 `alembic upgrade head && fastapi run` 手動執行。

### 6. Dashboard sidebar 處理

移除不存在的頁面連結（products, customers, settings），只保留 overview。同時將 sidebar labels 改用 `useTranslations('dashboard')` 取得 i18n 文字。

**理由**: 壞掉的連結比沒有連結更糟。未來新增頁面時再加回來。

## Risks / Trade-offs

- **[Resource limits 過低]** → 設定保守的預設值，附註文件說明如何根據 profiling 調整
- **[401 refresh 競態條件]** → 用 module-level flag + Promise 快取確保同一時間只有一個 refresh 請求
- **[Migration 只在 K8s init container 跑]** → 本地開發需手動跑 `alembic upgrade head`，在 backend Makefile 的 `run` target 已包含此步驟
- **[移除 sidebar 連結]** → 降低 dashboard 可見功能，但避免 404 體驗
- **[owner_id index migration]** → 需要在既有 table 上 CREATE INDEX，大表可能短暫 lock。目前資料量小，風險極低
