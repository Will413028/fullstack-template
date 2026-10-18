# 後端由 FastAPI 改寫成 Go

**Goal：** `backend/` 是 Go 1.27 的 modular monolith（stdlib `net/http`＋oapi-codegen strict server、pgx＋sqlc、Atlas 宣告式 schema、DB opaque session、`log/slog`）。對外 REST 契約只有兩處和舊版不同：刪掉 `/auth/refresh`、login body 改 JSON。前端改用單一 `session` cookie。CI、compose、`make rename`、指令檔與規則文件全部改為 Go。repo 內沒有 Python 後端殘留（步驟 8 的驗收）。

**上層：** 主清單在 repo 外，見本機個人脈絡檔。沒有主計畫，下一段暫代。

**附件：** [`2026-10-18-go-backend-rewrite/`](2026-10-18-go-backend-rewrite/)，快照 `7a4d0fa`：
- `inventory.md`：舊系統盤點。
- `test-map.md`：30 條舊測試與 Go 測試的對照。
- `check_inventory.py`：盤點完整性與殘留檢查。

決策紀錄：[`docs/adr/2026-10-18-go-backend.md`](../adr/2026-10-18-go-backend.md)。

## 全域約束與完成定義（暫代主計畫）

- **分支與 PR**：在分支 `go-rewrite` 上做。步驟 1 開 draft PR 到 `main`，之後每一步的「CI 綠燈」都以該 PR 的 `gh pr checks` 為準（CI 只在 PR 與 push `main` 觸發，`.github/workflows/ci.yml:3-7`）。不留 Python 與 Go 並行的路徑。
- **舊系統**：保存在 tag `archive/pre-go-rewrite`。要對照時用 `git worktree add <scratchpad>/ft-py archive/pre-go-rewrite`，執行方式見步驟 7。
- **依賴**：只用步驟 0 核准的清單。清單外的套件要另外列出、查 deps.dev 發布日期、等 Will 核准（`AGENTS.md` Constraints）。
- **錯誤與分層**：錯誤一律在 `internal/httpapi` 一處轉成 RFC 9457 `application/problem+json`（I4）。
- **SQL 與 schema**：每個 SQL 查詢都由 sqlc 生成。不手寫 migration：改 `schema.sql` 後用 `atlas migrate diff` 產生。
- **完成定義（每個模組）**：
  - 盤點表中該模組每一列的去向都已實作。
  - `test-map.md` 中該模組的每一列都有 Go integration 測試（testcontainers，PostgreSQL 18）。
  - 盤點 E 段屬於該模組的缺陷都有回歸測試。
  - 當下步驟定義的 `make -C backend check` 綠燈，draft PR 的 CI 綠燈。

## 前提（開工時核對過，2026-10-18）

- **P1 既有部署是否在線**：不在線，已確認。範本本身沒有部署：`.github/workflows/` 只有 `ci.yml`、`security.yml`，沒有 deploy。已 clone 出去的專案與範本斷開（專案頁 Overview）。所以不受回滾、相容性窗口、資料搬移約束，回滾就是不合併或 revert PR。
- **P2 動機與範圍**：對齊 FathomPod／dailyfresh 的 Go stack，直接取代 Python 後端。（Will，2026-10-18，已確認）
- **P3 前端呼叫面**：前端是唯一消費端，只呼叫 5 個 `/auth/*`；items CRUD 前端沒有用到，只在 `api-client.test.ts` 當 fixture。已確認：`ls frontend/src/features` 只有 `auth`，`rg -n "/items" frontend/src` 只命中測試。
- **P4 Go 版本與 module path**：Go 1.27（本機 `go1.27.2`、FathomPod `go 1.27.1`）。module path 用 `github.com/Will413028/fullstack-template/backend`（來自 `git remote -v`）。`make rename NAME=<n>` 預設改成 `github.com/Will413028/<n>/backend`；另加可選的 `MODULE=<path>` 覆寫，`NAME=` 介面不變。（預設（可推翻））
- **P5 HTTP 與契約來源**：（預設（可推翻））
  - HTTP 用 stdlib `net/http` `ServeMux`，加 oapi-codegen strict server，生成碼放在 `internal/httpapi/gen`。
  - 契約採 spec-first，`backend/api/openapi.yaml`（OpenAPI 3.0.3）是唯一來源。不用 3.1，因為 oapi-codegen 對 3.1 支援不完整。
  - breaking 檢查用 oasdiff 對 `main` 比對，從本 PR 合併後才生效。
- **P6 Session**：同 FathomPod identity，依 OWASP Session Management Cheat Sheet。（預設（可推翻））
  - token：32 bytes `crypto/rand`，base64url；DB 只存 SHA-256。
  - 期限：absolute 7 日、idle 1 日；`last_used_at` 每分鐘最多寫一次。
  - cookie：名稱 `session`，`HttpOnly`、`Path=/`；`Secure`、`SameSite`、`Domain` 依設定。
  - 登入一律發新 token，不沿用請求帶來的 token，以防 session fixation。
  - 非瀏覽器 client 可改用 `Authorization: Bearer <token>`。
- **P7 login body 改 JSON**：`POST /auth/login` 由 form-urlencoded `username`/`password` 改為 JSON `{account,password}`。form 只為 Swagger 的 OAuth2 password flow 存在，新文件頁不用那個 flow；前端只改一行。（預設（可推翻））
- **P8 密碼雜湊**：argon2id，m=19 MiB、t=2、p=1（OWASP Password Storage Cheat Sheet），存成 PHC 字串；無法解析的 hash 視為驗證失敗。（預設（可推翻））
- **P9 開發熱重載**：用 `docker compose watch`（`develop.watch` 的 `rebuild`），不加 air。（預設（可推翻））
- **P10 主鍵與 role 型別**：舊 schema 的 `id`、`role` 都是 int4（`alembic/versions/2024_05_16_0041-*.py:26,30`），新型別由 D6 決定。JSON 裡兩者仍是 number，所以契約不受影響。
- **P11 repo 工具維持 Python**：`scripts/*.py`（rename、init_env）是 repo 工具、不是後端，維持 Python。（預設（可推翻））
- **P12 本機 `.env` 由 Will 自己更新**：agent 不讀 `.env`。新變數依 `.env-example`；`DATABASE_URL` 去掉 `+asyncpg`；`SECRET_KEY` 可刪除。（需要人做，見步驟 1）

**驗收基線**

舊系統（`7a4d0fa`，2026-10-18 本機）：
- 後端：`uv run pytest -q` → `30 passed in 3.47s`（PostgreSQL 18 容器）。
- 前端：`pnpm test` → `Test Files 3 passed (3)、Tests 17 passed (17)`；`pnpm lint` → `Checked 39 files … No fixes applied.`
- 既有失敗：無。

新 Go 後端：尚未建立，基線在步驟 1 建立。

## 本階段依賴的不變量

| 不變量 | 依賴它的機制 |
|---|---|
| I1 對外 REST 形狀維持不變（P7 與 `/auth/refresh` 除外）：路徑；`{data}`／`{data,total,page,size,pages}`／`{detail}` 外殼；`UserResponse`、`ItemResponse` 的欄位名與 JSON 型別；PATCH 三態（盤點 B14） | 前端：`frontend/src/features/auth/api/auth-api.ts`、`features/auth/server.ts`、`features/auth/types.ts`、`lib/api-client.ts`<br>後端：`backend/api/openapi.yaml`<br>本次改寫靠 `parity.sh`（步驟 7）保護；oasdiff 從合併後才生效 |
| I2 錯誤 body 一定有 `detail`（字串）；驗證錯誤另有 `errors: {field: [msg]}` | `lib/api-client.ts` 的 `ApiError`：讀 `detail`、`errors`，不檢查 content-type<br>`internal/httpapi/problem.go` |
| I3 認證憑證只放在 `HttpOnly` cookie，JS 讀不到 | `internal/httpapi/auth.go`：cookie 寫入與清除<br>`frontend/src/proxy.ts`：只看 cookie 是否存在<br>`features/auth/server.ts`：轉發 cookie |
| I4 分層與依賴方向（細節見表下） | `.golangci.yml` depguard 規則；`internal/httpapi` 組裝；`cmd/app` |
| I5 schema 只由 Atlas 從各模組的 `schema.sql` 產生；auth 的 `schema.sql` 先載入；baseline 之後的 migration 只增不改 | `backend/atlas.hcl`（`src` 順序：auth、items）；`backend/migrations/`；`backend/scripts/check-schema-drift.sh`；`backend/scripts/check-migrations-immutable.sh`（寫法照 FathomPod 同名腳本）；root `AGENTS.md` Constraints |

**I4 的具體規則：**
- HTTP handler、cookie、oapi-codegen 生成碼都放在 `internal/httpapi`。
- `internal/modules/<m>` 只放 service、store 和 sqlc 生成碼，不 import `net/http`，也不 import 其他模組。
- 只有 `items.owner_id → users.id` 這一個跨模組 FK。items 的 `sqlc.yaml` 的 `schema` 同時列出 auth 與 items 的 `schema.sql`，但 items 的查詢不得 `SELECT` 或 `JOIN` `users`。
- 目前使用者經由 `internal/platform/authctx`（只用 `context.Context`）傳給模組。

## 規模（搬遷）

| | auth | items | 共用（core/health） | 產生計數的位置 |
|---|---|---|---|---|
| REST operation | 5（刪 1） | 5 | 2 | 盤點 A 段 |
| 表 | 2（`refresh_tokens` → `sessions`） | 1 | 0 | 盤點 C 段 |
| 舊測試 | 16 | 8 | 1＋5（config） | `test-map.md` |
| 既有缺陷 | E2、E3、E5、E8、E10 | E1、E6、E9 | E4、E7 | 盤點 E 段 |

## 範圍

- **做**：盤點 A–F 段全部列；E 段 10 項缺陷全修。專案頁 Pending 的處置照 D5 已在 2026-10-18 完成（見 D5）。
- **不做**：
  - 前端 OpenAPI 型別生成。理由：前端只有 auth 一個 feature，生成工具又是新依賴，收益小。
  - 背景工作框架（River）、OAuth／SSO。
- **延後**：
  - 前端從 `openapi.yaml` 生成型別。觸發：前端加第二個呼叫後端的 feature。
  - 背景工作 River。觸發：下游專案或範本出現第一個非同步工作。
  - 代管認證或 MFA。觸發：需要 MFA、SSO 或第三方登入。
  - 多實例 rate limit 改用 DB 或 proxy 層。觸發：backend 跑超過一個 replica。

## 步驟

- [ ] **0. 依賴清單與核准**（被擋於：無）
  - 範圍：列出每項的確切版本、發布日期與 deps.dev（或 GHCR／Docker Hub）連結；30 天內才發布的版本不採用。清單交 Will 核准：
    - Go module：
      - `github.com/jackc/pgx/v5`
      - `github.com/oapi-codegen/runtime`、`github.com/oapi-codegen/nullable`（PATCH 三態）
      - `github.com/getkin/kin-openapi`：只在用 oapi-codegen 的 request 驗證 middleware 時才需要
      - `golang.org/x/crypto`（argon2）、`golang.org/x/time`（rate）
      - `github.com/testcontainers/testcontainers-go` 及其 `modules/postgres`
    - 工具：用 `go tool` 或在 CI 安裝，鎖版本。
      - oapi-codegen、sqlc、Atlas CLI、golangci-lint v2、govulncheck、oasdiff、`github.com/boumenot/gocover-cobertura`
      - squawk-cli：npm，`npx --yes squawk-cli@<ver>`
      - diff-cover：PyPI，CI 用 `pipx run diff-cover==<ver>`
    - GitHub Actions：`actions/setup-go`，鎖 SHA（zizmor 會檢查）。
    - Docker image：`golang:1.27-bookworm`（builder）、`gcr.io/distroless/static-debian12`（runtime），都鎖 digest；Atlas 二進位的下載 URL 與 checksum。
    - dev 文件頁：`swagger-ui-dist`，從 jsDelivr 載入並鎖版本，只在 `MODE=dev` 時提供。
    - 前端：無新依賴。
  - 消費端：無（只產出清單）。
  - 不能動：任何檔案。
  - 驗收：清單每一列都有版本、發布日期、連結；Will 回覆核准。
  - 停止條件：
    - 某套件沒有符合條件的版本 → 回報替代方案。
    - Atlas community 版不支援 `docker://` dev-url 或 `concurrent_index` policy → 回報。FathomPod 下載的是非 community 的 `atlas-linux-amd64-v1.1.3`，要換版本前先問。
  - 需要人做的事：Will 核准清單。
- [ ] **1. Walking skeleton**（被擋於：步驟 0）
  - 範圍：
    - 分支、tag 與 PR：開分支 `go-rewrite`；在開工時的 `main` 打 tag `archive/pre-go-rewrite` 並 push；開 draft PR。
    - 刪除 Python 後端：`backend/src`、`tests`、`alembic*`、`pyproject.toml`、`uv.lock`、`scripts/wait_for_db.py`、`.python-version`（若有）。
    - 程式骨架：
      - `go.mod`，`cmd/app` 先只有子指令 `serve`。
      - `internal/platform/config`：盤點 D 段與 E10。
      - `internal/platform/logx`：slog。
      - `internal/httpapi`：`GET /health` 與 `GET /health/ready`（含 E7 的 503）。
    - Atlas：`atlas.hcl`、空的 `migrations/`。
    - 建置與部署：
      - `backend/Dockerfile`：multi-stage，內附 Atlas CLI 給 migrate service。
      - `docker-compose*.yml`（F5）：migrate 改跑 `atlas migrate apply`，dev 改用 watch。
      - `backend/Makefile`：`check` 先包含 gofmt、golangci-lint、`go test -race`、govulncheck。
      - root `Makefile` 的 `migration`、`generate-migration`（F4 前半）。
      - `.env-example`（F6）。
    - CI 與 lint：
      - CI 的 Go job（F1）先只有 lint、test、govulncheck。
      - `.golangci.yml`，含 I4 的 depguard 規則。
  - 消費端：`rg -n "backend" docker-compose.yml docker-compose.dev.yml Makefile .github/workflows/ci.yml .env-example`
  - 不能動：`frontend/`；root `Makefile` 的 `seed`、`reset-db`（步驟 3 才改，這段期間暫時不能用）。
  - 驗收：
    - `make -C backend check` 綠燈。
    - `docker compose up -d --build` 後：
      - `curl -fsS localhost:8000/health` 回 `{"status":"ok"}`；
      - `curl -fsS localhost:8000/health/ready` 回 `{"status":"ready"}`；
      - migrate service 以 exit 0 結束。
    - `docker compose stop postgres` 後，`/health/ready` 回 503 problem+json。mutation：把 503 改回 500，`TestReadyDBDown` 要失敗。
    - config 測試照 `test-map.md` 前 5 列。mutation：拿掉「SameSite=None 需 Secure」的檢查，`TestConfigSameSiteNoneRequiresSecure` 要失敗。
    - draft PR 的 `gh pr checks` 全綠（Go job；frontend job 不受影響）。
  - 停止條件：
    - `atlas migrate apply` 遇到空目錄時以非 0 結束 → compose 的 migrate service 先跳過 Atlas，直接 `exit 0`，在步驟 3 補上。這個決定要寫進進度表。
    - Atlas CLI 無法在 distroless 中執行 → 回報；改用 Debian slim 前先問。
  - 需要人做的事：Will 依 P12 更新本機 `.env`。
  - 完成後的狀態：前端登入暫時不能用（auth 在步驟 3 完成）。步驟 1–5 必須全部完成才能合併。
- [ ] **2. 共用平台**（被擋於：步驟 1）
  - 範圍：
    - 契約與生成：`api/openapi.yaml` 骨架，含共用 schema `DataResponse`、`PaginatedResponse`、`DetailResponse`、`Problem`；oapi-codegen 生成到 `internal/httpapi/gen`。
    - 錯誤：`internal/platform/apperr`（kind → status）；`internal/httpapi/problem.go`：problem+json 寫出，未處理錯誤回 500 並記 log，含 panic recover。
    - middleware：request id（E5）、安全標頭（E4）、CORS（B4）、access log（含 `duration_ms`）、rate limiter（B6、E3，`TRUSTED_PROXIES`、`AUTH_RATE_LIMIT_PER_MIN`）。
    - 分頁解析：`internal/platform/httpx`（B9）。
    - 資料層工具：sqlc 設定、squawk、`check-schema-drift.sh`、`check-migrations-immutable.sh`、Atlas 的 `concurrent_index` policy。
    - 測試 harness：testcontainers，每個測試套件一個 PG 18 容器，套用 `migrations/`。
    - dev 模式文件：`GET /openapi.yaml` 與 Swagger UI。
    - 檢查與 CI：`make check` 加上 sqlc diff、漂移、squawk、migrations-immutable；CI 加上 oasdiff（`main` 沒有 `openapi.yaml` 時跳過）與 coverage → Cobertura → diff-cover。
  - 消費端：無外部消費端（模組在步驟 3、4 才接上）。
  - 不能動：I2 的 `detail`／`errors` 欄位名。
  - 驗收（各條測試對應 `test-map.md` 的新增列）：
    - 驗證錯誤的 problem body 同時有 `detail` 與 `errors`。mutation：把 `errors` 改名 `fieldErrors`，測試要失敗。
    - 非法的 request id 被換掉。mutation：拿掉格式檢查，測試要失敗。
    - 安全標頭 `X-XSS-Protection: 0`。
    - rate limit：
      - 來源在 `TRUSTED_PROXIES` 內時，兩個不同 XFF 各自計數；來源不受信任時忽略 XFF。mutation：一律採用 XFF，測試要失敗。
      - 超限回 429，並帶 `Retry-After`。
    - CORS 預檢回 `Access-Control-Allow-Credentials: true` 與允許的方法。
    - handler panic 時回 500 problem+json。
    - depguard：在 `internal/modules/` 下暫放一個 import `net/http` 的檔案，`golangci-lint run` 要失敗；確認後刪除該檔。
    - 整體：`make -C backend check` 綠燈、PR CI 綠燈。
- [ ] **3. auth 模組**（被擋於：步驟 2、D6）
  - 範圍：
    - `internal/modules/auth`：
      - `schema.sql`（盤點 C1、C2，型別依 D6）與 queries，經 sqlc 生成。
      - service：register（建 user 與 session 在同一 transaction）、login、logout、current user。依 B10、B12、B15、P6、P8。
    - `internal/httpapi/auth.go`：handler 與 cookie（A3、A4、A6、A7）。
    - `internal/platform/authctx`：把目前使用者放進 context，供其他模組取用。
    - baseline migration：`atlas migrate diff baseline`。
    - seed：`cmd/app seed-admin`（E8）。
    - root `Makefile` 的 `seed`、`reset-db`（F4 後半，`reset-db` 改成輪詢 `/health/ready`）。
    - 若步驟 1 跳過了 migrate service 的 Atlas，在這一步補上。
  - 消費端：`rg -n "/auth/" frontend/src`
  - 不能動：
    - I1（`/auth/refresh` 除外）、I3。
    - timing 防護：未知帳號仍跑一次 argon2 驗證。
    - B15：disabled 帳號 login 401、`/auth/me` 401。
  - 驗收：
    - `test-map.md` 的 auth 16 列與新增列都通過。
    - mutation，各自要讓指定測試失敗：
      - (a) 拿掉 dummy hash 分支 → `TestLoginUnknownAccountRunsHash`。
      - (b) 登入時沿用請求帶來的 token → `TestLoginRotatesSession`。
      - (c) 拿掉 idle 檢查 → `TestSessionIdleExpiry`。
      - (d) 拿掉 `is_disabled` 檢查 → `TestLoginDisabledAccount`、`TestMeDisabledAccount`。
    - 手動 curl：register → me 200 → logout → me 401。
    - PR CI 綠燈。
- [ ] **4. items 模組**（被擋於：步驟 3）
  - 範圍：
    - `internal/modules/items`：`schema.sql`（C3），`sqlc.yaml` 的 schema 照 I4；queries、service。
    - `internal/httpapi/items.go`：A8–A11。
    - migration 由 `atlas migrate diff` 產生（第二支）。
  - 消費端：`rg -n "/items" frontend/src`（預期只命中 `lib/__tests__/api-client.test.ts`）
  - 不能動：I1 中 items 的形狀，包含 B14；分頁參數語意（B9）。
  - 驗收：
    - `test-map.md` 的 items 8 列與 `TestCreateItemTitleTooLong` 都通過。
    - mutation，各自要讓指定測試失敗：
      - (a) 拿掉 owner 條件 → `TestGetOtherUsersItem`。
      - (b) PATCH 的 request 型別改成一般指標 → `TestUpdateItemTriState`。
      - (c) 拿掉 `ORDER BY id` → `TestPagination`。
    - `rg -n "users" internal/modules/items/queries` 無結果（I4）。
    - PR CI 綠燈。
  - 完成後的狀態：後端完成。前端仍呼叫 `/auth/refresh` 並用 form login，必須接著做步驟 5。
- [ ] **5. 前端切換**（被擋於：步驟 3；與步驟 4 一起合併）
  - 範圍：盤點 F16–F22。
  - 消費端：`rg -n "refresh|access_token|refresh_token|URLSearchParams|form-encoded" frontend/src frontend/AGENTS.md`
  - 不能動：I3；`getServerUser()` 仍是真正的認證關卡（`frontend/AGENTS.md`）。
  - 驗收：
    - `pnpm lint`、`pnpm test`、`pnpm build` 綠燈。
    - 新測試：收到 401 時直接導向 login，不呼叫 `/auth/refresh`。mutation：放回 refresh 重試，測試要失敗。
    - `proxy.ts` 測試：有 `session` cookie 才放行 protected path。
    - 端到端：`docker compose up --build` 後，用瀏覽器（Claude in Chrome）走完 register → overview → logout → 被導回 login，並錄 GIF。
    - PR CI 綠燈。
- [ ] **6. 外圍與文件**（被擋於：步驟 4、D6）
  - 範圍：
    - 盤點 F2、F3、F7–F15，以及 F21、F22。
    - `rename.py`：P4 的 module path 規則（`MODULE=` 為可選），以及 F12 的 `CLAUDE.md` → `AGENTS.md`。
    - root `AGENTS.md`：Verification 改為 `make -C backend test`（Go）與 `pnpm test`；Constraints 的 migration 規則改成 I5 的寫法。
    - `docs/rules/*`：依 D6 改寫，範例換成 Go 與 SQL，錯誤格式改成 RFC 9457。
    - `.claude/skills/add-module.md`、`add-crud.md`：改寫成 Go 骨架。
  - 消費端：`python3 -I docs/plans/2026-10-18-go-backend-rewrite/check_inventory.py --after-rewrite`
  - 不能動：`make rename` 的 `NAME=` 介面。
  - 驗收：
    - rename：在 scratchpad 的 clone 跑 `make rename NAME=acme`：
      - `head -1 backend/go.mod` 等於 `module github.com/Will413028/acme/backend`；
      - `rg -n "fullstack-template/backend" backend` 無結果；
      - `go build ./...`、`go test ./...`、`pnpm build` 綠燈；
      - 另跑一次 `MODULE=example.com/x/backend`，`go.mod` 第一行要對應改變；
      - mutation：rename 不改 import，`go build` 要失敗。
    - `check_inventory.py --after-rewrite` 輸出 `OK`。
- [ ] **7. 行為等價**（被擋於：步驟 4、5）
  - 範圍：寫 `docs/plans/2026-10-18-go-backend-rewrite/parity.sh`。
    - 兩套系統各用獨立的 PG 18 容器。
    - 舊系統：在 tag worktree 執行 `uv sync`、`alembic upgrade head`，再用 `python -c "from src.auth.router import limiter; limiter.enabled=False; import uvicorn; from src.main import app; uvicorn.run(app, port=8001)"` 啟動（關掉 rate limit 的方式同 `tests/conftest.py:91-93`）。環境變數：`DATABASE_URL`、`SECRET_KEY`（測試值）、`COOKIE_SECURE=false`。
    - 新系統：`AUTH_RATE_LIMIT_PER_MIN=10000`、`COOKIE_SECURE=false`。
    - 請求：register、重複 register、弱密碼、login 成功與失敗、me、items 的建立／列表／分頁／讀／改／刪、讀他人 item、未登入。兩邊各送一次。
    - 比對：status 與 JSON 的 key 結構；id 與時間戳先正規化再比。
    - 白名單：盤點列出的刻意差異（A4 body、A5 刪除、A10/A11 改 404、B2 的 problem 欄位、E 段）。429 不列入白名單。
  - 消費端：無。
  - 驗收：
    - `parity.sh` 的差異全部在白名單內。
    - mutation：把新系統 `UserResponse` 的 `is_disabled` 改名，`parity.sh` 要報出差異。
- [ ] **8. 收尾**（被擋於：步驟 1–7）
  - 完成定義逐項核對。
  - design-review skill（獨立 subagent）。
  - `check_inventory.py --after-rewrite` 輸出 `OK`。
  - 長期檢查都已接在 CI：depguard、漂移、migrations-immutable、oasdiff。
  - code review。
  - PR 轉成 ready；CI 綠燈後合併（範本沒有部署，合併不算 deploy）。
  - 計畫與附件不留在範本裡，因為每個下游 clone 都會帶走：本計畫檔與附件目錄在 PR 的最後一個 commit 刪除，進度摘要寫進 PR 內文與專案頁。ADR 保留在 `docs/adr/`，對下游也有說明價值。
  - 延後項附觸發條件搬回專案頁 `## Pending`，並刪除本計畫的掛載項。
  - 驗收：`gh pr checks` 全綠的輸出；design-review 的每個發現都有處置。

## 決定

- **D1 範圍**（已決 2026-10-18，Will；擋住：1）
  - 直接取代：刪除 Python，舊系統打 tag 保存。
  - ADR：`docs/adr/2026-10-18-go-backend.md`（下同）。
- **D2 對外契約**（已決 2026-10-18，Will；擋住：2–5）
  - REST＋OpenAPI，保留現有路徑與回應外殼；錯誤改 RFC 9457。
  - 重新評估的觸發：新專案預設會有原生 app。
- **D3 資料層與 migration**（已決 2026-10-18，Will；擋住：2–4）
  - pgx＋sqlc＋Atlas 宣告式。理由：Will 偏好不手寫 SQL migration（同 FathomPod 2026-10-07 的決定）。
- **D4 認證**（已決 2026-10-18，Will；擋住：3、5）
  - DB opaque session，參數見 P6。
  - 重新評估的觸發：需要 MFA、SSO 或第三方登入。
- **D5 專案頁 Pending 中的 Python 後端項目**（已決 2026-10-18，Will）
  - 移除。2026-10-18 已在專案頁逐行處置：只屬 Python 的刪除；前後端混合的項目保留前端部分，後端部分改成 Go 對應項，併入本計畫。
- **D6 DB 型別規則在 Go 版的寫法**（未決；擋住：3、4、6；需要的事實：無）
  - **問題**：`docs/rules/database-design.md` 是強制規則。它要求短文字用 `String(N)`、狀態欄不加 CHECK、主鍵用 `int` 自增。改寫時要決定沿用這些規則，還是改成 PostgreSQL 社群的建議做法。
  - **選項 A：沿用現行規則，只換成 SQL 寫法**：`varchar(N)`、狀態欄不加 CHECK、`integer generated always as identity`。代價：不符合 PG wiki 的「Don't Do This」（不建議用 `varchar(n)` 當預設）；int4 主鍵約 21 億筆就會用完。
  - **選項 B：照 PG wiki「Don't Do This」**：文字欄用 `text`，有長度限制時加 `CHECK (char_length(x) <= N)`；主鍵用 `bigint generated always as identity`；狀態欄仍不加 CHECK，維持現行規則的理由（只有一個 app 寫 DB，值由 Go 常數驗證）。代價：要改寫強制規則的型別表。
  - **選項 C：B，另外狀態欄也加 CHECK**（同 FathomPod）。代價：每加一種狀態，Go 常數和 DB constraint 兩處都要改。
  - **建議：B。** 理由：`text` 與 `bigint identity` 是 PG 社群的基準；狀態欄要不要加 CHECK，現行規則有它自己成立的理由，不必推翻。
  - 結論：待 Will 決定；決定後寫進 ADR。

拍板順序：D6 現在就可以拍，不影響步驟 0–2 先做。P4–P11 是預設，Will 可推翻；推翻時依 phase-plan 步驟 5 重寫受影響的步驟。

## 進度

狀態值：未開始／進行中／程式完成待驗收／完成。

| 步驟 | 狀態 | 已跑的驗收 | 未跑的驗收與原因 | commit |
|---|---|---|---|---|
| 0 依賴核准 | 未開始 | | | |
| 1 Walking skeleton | 未開始 | | | |
| 2 共用平台 | 未開始 | | | |
| 3 auth | 未開始 | | | |
| 4 items | 未開始 | | | |
| 5 前端切換 | 未開始 | | | |
| 6 外圍與文件 | 未開始 | | | |
| 7 行為等價 | 未開始 | | | |
| 8 收尾 | 未開始 | | | |
