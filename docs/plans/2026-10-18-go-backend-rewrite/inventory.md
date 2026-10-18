# 舊後端盤點（FastAPI → Go）

快照 commit：`7a4d0fa`（2026-10-18）。舊系統就是規格；每列的「去向」在收尾時不得有空格（`check_inventory.py` 檢查）。

產生方式：

```bash
# REST operation
rg -n '@router\.(get|post|patch|delete|put)' backend/src
# 表
rg -n '__tablename__' backend/src
# 設定
rg -n '^    [A-Z_]+: ' backend/src/core/config.py
# 後端以外引用 Python 後端的檔案（消費端；排除 node_modules 與 lockfile）
rg -l -i 'backend|python|uv |pytest|ruff|alembic|diff-cover|fastapi|sqlalchemy|pydantic' \
  -g '!backend/**' -g '!frontend/node_modules/**' -g '!**/pnpm-lock.yaml' -g '!docs/plans/**' --hidden -g '!.git/**'
```

## A. REST operation（11 個）

| # | Operation | 舊行為 | 去向 |
|---|---|---|---|
| A1 | `GET /health` | `{"status":"ok"}` | 保留，一般 handler，不進 OpenAPI 的業務 tag |
| A2 | `GET /health/ready` | `SELECT 1`，成功回 `{"status":"ready"}`，失敗時 500 | 保留成功 body；DB 失敗回 503 problem+json（E7） |
| A3 | `POST /auth/register` | JSON `{account,password}`；建 user、發 access JWT＋refresh token cookie；回 `{data: UserResponse}`；5/min/IP | 改寫：建 user＋建 session（同一 transaction）、設 `session` cookie；回應形狀不變；rate limit 保留 |
| A4 | `POST /auth/login` | `application/x-www-form-urlencoded` `username`/`password`（OAuth2PasswordRequestForm）；dummy hash 防 timing；disabled 視同失敗；5/min/IP | 改寫：body 改 JSON `{account,password}`（form 只為 Swagger OAuth2 password flow 存在，見計畫前提 P7）；session cookie；timing 防護與 disabled 行為保留 |
| A5 | `POST /auth/refresh` | refresh token 輪替、撤銷舊 token | 刪除：opaque session 沒有 access/refresh 之分（D4）；idle 期限由每請求的 `last_used_at` 延長 |
| A6 | `POST /auth/logout` | 撤銷 refresh token、清 cookie；回 `{"detail":"Logged out"}` | 改寫：刪除 session 列、清 cookie；回應形狀不變 |
| A7 | `GET /auth/me` | cookie 或 `Authorization: Bearer` JWT；disabled → 401 | 改寫：cookie 或 `Authorization: Bearer <session token>`；disabled → 401 |
| A8 | `POST /items` | 建 item（owner = 目前使用者）；背景 task 記 log | 改寫；log 直接同步寫（BackgroundTasks 只為示範存在，見 B 段） |
| A9 | `GET /items` | 目前使用者的 items，`page`≥1、`size` 1–100，回 `{data,total,page,size,pages}` | 改寫；形狀不變；加穩定排序 `ORDER BY id`（E6） |
| A10 | `GET /items/{id}` | 任何登入者可讀任何 item | 改寫；非 owner 回 404（E1） |
| A11 | `PATCH /items/{id}` / `DELETE /items/{id}` | owner 檢查，非 owner 403；DELETE 回 `{"detail":"Item deleted"}` | 改寫；非 owner 改回 404（與 E1 一致，不洩漏存在與否）；形狀不變 |

開發模式的 `/docs`、`/redoc`、`/openapi.json`：改寫，dev 模式提供 `GET /openapi.yaml`（契約檔原樣）與 Swagger UI 靜態頁（`MODE=dev` 才掛）。

## B. 跨切面行為

| # | 舊行為 | 位置 | 去向 |
|---|---|---|---|
| B1 | 回應包 `{data}`／`{data,total,page,size,pages}`／`{detail}` | `core/schemas/base.py` | 保留 |
| B2 | 錯誤 `{"detail": str}`；驗證錯誤 422 `{"detail":"Validation failed","errors":{field:[msg]}}`；未處理 500 `{"detail":"Internal server error"}` | `core/exception_handlers.py` | 改寫成 RFC 9457 `application/problem+json`：`type`、`title`、`status`、`detail`；驗證錯誤另帶擴充成員 `errors: {field: [msg]}`；前端 `ApiError` 讀 `detail` 與 `errors`，不需改解析 |
| B3 | `AppException` 家族（404/409/401/403） | `core/exceptions.py` | 改寫：`internal/platform/apperr` 的 kind，在 HTTP 邊界一處轉 status |
| B4 | middleware：`X-Request-ID`（沿用 client 值）、`X-Process-Time`、安全標頭五個、CORS（credentials、方法、`Content-Type`/`Authorization`） | `core/middleware.py`、`main.py` | 保留 request id（驗證格式，E5）、安全標頭（`X-XSS-Protection` 改 `0`，E4）、CORS；`X-Process-Time` 刪除（改寫進 access log 的 `duration_ms`，避免對外洩漏時間資訊） |
| B5 | structlog：dev console、prod JSON，綁 request_id | `core/logging.py` | 改寫：`log/slog`，dev text、prod JSON，request_id 由 context 帶入 |
| B6 | rate limit：slowapi in-memory、key = 連線 IP、4 個 auth 寫入 endpoint 5/min | `auth/router.py` | 改寫：`x/time/rate` per-IP in-memory，套 register/login/logout；IP 依 `TRUSTED_PROXIES` 解析 `X-Forwarded-For`（E3）；超限回 429 problem+json＋`Retry-After` |
| B7 | DB session per request、成功 commit、例外 rollback | `core/database.py` | 改寫：pgxpool；需要多語句原子性的 service 方法自己開 transaction（register：建 user＋session） |
| B8 | `BaseRepository[T]` 泛型 CRUD | `core/repository/base.py` | 刪除：sqlc 每查詢生成函式（Will 2026-10-18 拍板決策 3） |
| B9 | `PaginationParams`（page≥1、size 1–100，預設 1/20） | `core/dependencies.py` | 保留語意，`internal/platform/httpx` 的解析函式；違規回 422 problem+json |
| B10 | argon2（pwdlib recommended）；未知 hash → 驗證失敗不 500 | `core/security.py` | 保留：argon2id，參數照 OWASP Password Storage Cheat Sheet（m=19MiB, t=2, p=1）；PHC 字串格式 |
| B11 | `BackgroundTasks` 示範 | `items/router.py` | 刪除：只是示範 log；Go 範本不示範背景工作（要時另加 River，見計畫延後） |
| B12 | 密碼規則：8–100 字、含大小寫與數字；account 3–50 字 | `auth/schemas.py` | 保留，寫進 OpenAPI schema 並在 handler 驗證 |
| B13 | `UserResponse {id:int, account, is_disabled, role:int}`；`ItemResponse {id, title, description|null, owner_id, created_at, updated_at}` | schemas | 保留形狀 |
| B14 | `PATCH /items/{id}` 三態：欄位沒送＝不改、`description: null`＝清成 NULL（`model_dump(exclude_unset=True)`） | `items/service.py` `update_item` | 保留：request 型別用 `nullable.Nullable[T]`（oapi-codegen），不用一般指標；`title: null` 依 E9 回 422 |
| B15 | disabled 帳號：login 401（與密碼錯誤同訊息）、`/auth/me` 401 | `auth/service.py:50`、`auth/dependencies.py` | 保留；舊測試沒有覆蓋，Go 補測試 |

## C. 表（3 張）與 migration（4 支）

| # | 舊 | 去向 |
|---|---|---|
| C1 | `users(id serial, account varchar(50) unique, password varchar(255), is_disabled bool, role int default 3, created_at, updated_at)`（`id`、`role` 皆 int4，見 `alembic/versions/2024_05_16_0041-*.py:26,30`） | 改寫，欄位型別依計畫 D6：`id` identity、`account` 唯一、`password` 改名 `password_hash`、`is_disabled`、`role`（JSON 仍是 number）、`timestamptz`；`updated_at` 由 sqlc 查詢明寫 `updated_at = now()` |
| C2 | `refresh_tokens(token_hash, user_id, expires_at, revoked, created_at)` | 刪除，改為 `sessions(id bigint identity, token_hash bytea unique, user_id fk → users ON DELETE CASCADE, created_at, last_used_at, expires_at)`；FK 欄位建索引 |
| C3 | `items(id, title varchar(200), description text null, owner_id fk, created_at, updated_at)`、`idx` on owner_id | 改寫，型別依 D6；`owner_id` FK 指向 auth 模組的 `users`（唯一允許的跨模組 FK，計畫 I4）、FK 索引保留 |
| C4 | 4 支 Alembic migration | 刪除：範本無部署、無資料（計畫前提 P1），以 Atlas 產生的單一 baseline migration 取代 |

## D. 設定（環境變數）

| # | 舊 | 去向 |
|---|---|---|
| D1 | `APP_NAME`、`APP_VERSION`、`MODE`（dev/prod） | 保留 `APP_NAME`、`MODE`；`APP_VERSION` 刪除（改由 build 時 `-ldflags` 注入） |
| D2 | `DATABASE_URL`（`postgresql+asyncpg://` 自動轉換） | 保留，接受 `postgres://`／`postgresql://`；遇到 `+asyncpg` 等 driver 後綴時啟動失敗並提示改法（不靜默轉換，避免舊 `.env` 混用） |
| D3 | `SECRET_KEY`（≥32、prod 擋 `change-me`）、`ALGORITHM`、`ACCESS_TOKEN_EXPIRE_MINUTES`、`REFRESH_TOKEN_EXPIRE_DAYS` | 刪除：opaque session 不簽章；改 `SESSION_ABSOLUTE`（預設 `168h`）、`SESSION_IDLE`（預設 `24h`）；`scripts/init_env.py` 不再產 SECRET_KEY（改產 `POSTGRES_PASSWORD`，若目前沒有就只移除該段） |
| D4 | `COOKIE_SECURE`、`COOKIE_SAMESITE`（驗證值、none 需 secure）、`COOKIE_DOMAIN` | 保留，驗證規則保留 |
| D5 | `DB_POOL_SIZE`、`DB_MAX_OVERFLOW`、`DB_POOL_RECYCLE` | 改寫：`DB_MAX_CONNS`（預設 20）；其餘刪除（pgxpool 用 `MaxConnLifetime` 預設） |
| D6 | `CORS_ORIGINS` | 保留（逗號分隔） |
| D7 | — | 新增：`TRUSTED_PROXIES`（CIDR 清單，預設空＝不信任 XFF）、`AUTH_RATE_LIMIT_PER_MIN`（預設 5，parity 與壓測時調高）、`ADMIN_ACCOUNT`／`ADMIN_PASSWORD`（seed 用，E8） |

## E. 既有缺陷

| # | 缺陷 | 證據 | 處置 |
|---|---|---|---|
| E1 | `GET /items/{id}` 沒有 owner 檢查，任何登入者可讀他人 item（IDOR） | `items/service.py` `get_item` 不帶 owner；`test_get_item` 只測自己的 | 修：非 owner 回 404；回歸測試：使用者 2 讀使用者 1 的 item 要 404 |
| E2 | access token 有效但 user 已刪除時 `/auth/me` 回 404 而非 401 | `auth/service.py:get_user_by_account` 拋 NotFound | 修：session 查不到 user（CASCADE 刪 session）即 401；回歸測試：刪 user 後帶舊 cookie 呼叫 `/auth/me` 要 401 |
| E3 | rate limit key 是連線 IP；部署走 Caddy（`docs/deploy-vps.md` §5）時全站共用一個 5/min bucket | slowapi `get_remote_address` | 修：`TRUSTED_PROXIES` 內的來源才採 XFF 最右一個不受信任位址；回歸測試：兩個不同 XFF 的請求各自計數、未受信任來源的 XFF 被忽略 |
| E4 | `X-XSS-Protection: 1; mode=block` 已被 OWASP 建議關閉（舊瀏覽器有 XS-Leak） | `core/middleware.py` | 修：改 `0`；測試斷言標頭值 |
| E5 | `X-Request-ID` 原樣採用 client 值，可注入任意字串到 log | `core/middleware.py` | 修：只接受 ≤64 字的 `[A-Za-z0-9-]`，否則重新產生；回歸測試 |
| E6 | `GET /items` 分頁沒有 ORDER BY，跨頁結果不穩定 | `items/repository.py` | 修：`ORDER BY id`；`test_pagination` 斷言順序 |
| E7 | `/health/ready` DB 失敗走未處理例外回 500 | `health/router.py` | 修：回 503；測試用關閉的 pool |
| E8 | `make seed` 建固定帳密 `admin`／`Admin123` | `Makefile:72-88` | 修：`app seed-admin` 讀 `ADMIN_ACCOUNT`／`ADMIN_PASSWORD`，未設就失敗；密碼套 B12 規則 |
| E9 | `ItemCreate.title` 無長度限制、`ItemUpdate.title` 可為 `null`，超長或 null 進 NOT NULL 欄位會 500 | `items/schemas.py` 對 `String(200)` 非 null | 修：title 必填 1–200 字，PATCH 的 `title: null` 回 422；回歸測試 |
| E10 | `test_config.py` 的 5 條設定驗證：3 條 SameSite、2 條 `SECRET_KEY` | `tests/test_config.py:16-35` | SameSite 3 條移植；`SECRET_KEY` 2 條隨 D3 刪除，改測 `SESSION_IDLE` ≤ `SESSION_ABSOLUTE` 與 `DATABASE_URL` 帶 `+asyncpg` 時失敗 |

## F. 消費端（後端以外引用 Python 後端的檔案）

指令見檔頭第四條（2026-10-18 命中 31 個，含本次新增的 ADR）；`.claude/skills/*` 含在內。`backend/` 內的 `AGENTS.md`、`README.md`、`Makefile`、`Dockerfile` 在搜尋排除範圍外，列在 F8 與步驟 1。

| # | 檔案 | 去向 |
|---|---|---|
| F1 | `.github/workflows/ci.yml` | 改寫：Go job（golangci-lint、`go test -race`、coverage → Cobertura → diff-cover、govulncheck、sqlc diff、Atlas schema 漂移與 squawk） |
| F2 | `.github/dependabot.yml`（`uv`） | 改寫：`gomod`＋`docker`（backend）；`uv` 刪除 |
| F3 | `lefthook.yml` | 改寫：pre-commit 跑 `gofmt`/golangci-lint（changed files） |
| F4 | `Makefile`（root） | 改寫：`migration`、`generate-migration` 改 Atlas（步驟 1）；`seed`、`reset-db`（呼叫 `wait_for_db.py`）改 `app seed-admin` 與 `/health/ready` 輪詢（步驟 3）；其餘保留 |
| F5 | `docker-compose.yml`、`docker-compose.dev.yml` | 改寫：migrate service 跑 `atlas migrate apply`；dev 用 `docker compose watch` 的 `rebuild`（計畫前提 P9） |
| F6 | `.env-example`、`frontend/.env.example` | 改寫：依 D 段 |
| F7 | `AGENTS.md`（root） | 改寫：Verification（`make test` → Go）、Conventions、Structure |
| F8 | `backend/AGENTS.md`、`backend/README.md`、`backend/Makefile`、`backend/Dockerfile` | 改寫成 Go 版（`Makefile`、`Dockerfile` 在步驟 1，文件在步驟 6） |
| F9 | `docs/rules/api-design.md`、`docs/rules/database-design.md` | 改寫：規則保留（URL、回應形狀、命名、FK 不 cascade 預設），範例換 Go／SQL，錯誤格式改 RFC 9457 |
| F10 | `docs/deploy-vps.md` | 改寫提到 Python／backend 映像與 migration 的段落 |
| F11 | `README.md` | 改寫 |
| F12 | `scripts/rename.py` | 改寫：`go mod edit -module` ＋ 改寫所有 import 前綴＋`go build ./...` 驗證；`pyproject.toml` 那列刪除；`brand_files` 的 `CLAUDE.md` 改 `AGENTS.md`（專案頁 Pending 的文件項） |
| F13 | `scripts/init_env.py` | 改寫：依 D3 |
| F14 | `.claude/skills/add-module.md`、`.claude/skills/add-crud.md` | 改寫成 Go 模組骨架（schema.sql、queries、sqlc、handler、OpenAPI 片段） |
| F15 | `.gitignore` | 改寫：Python 項刪除，加 Go 產物 |
| F16 | `frontend/src/lib/api-client.ts` | 改寫：刪 401 自動 refresh 與 mutex（A5 刪除） |
| F17 | `frontend/src/proxy.ts` | 改寫：檢查 `session` cookie；刪 refresh 轉發 |
| F18 | `frontend/src/features/auth/api/auth-api.ts`、`frontend/src/features/auth/hooks/use-auth.ts` | 改寫：login 送 JSON；刪 `refresh` |
| F19 | `frontend/src/features/auth/server.ts` | 保留（只呼叫 `/auth/me`、轉發 cookie）；註解更新 |
| F20 | `frontend/src/features/auth/__tests__/server.test.ts`、`frontend/src/lib/__tests__/api-client.test.ts` | 改寫：刪 refresh 測試，加「401 直接導向 login」測試 |
| F21 | `frontend/src/app/layout.tsx:10,14,22`、`frontend/messages/en.json:19`、`frontend/messages/zh-TW.json:19` | 改寫：描述「FastAPI」→「Go」（「k3s」同步改成實際的 Docker Compose） |
| F22 | `frontend/AGENTS.md:14,55,65-78` | 改寫：auth 流程（JSON login、單一 `session` cookie、無 refresh） |
| F23 | `frontend/README.md:14`、`docs/frontend-known-issues.md:15,17` | 保留：只說「backend 設 cookie」，與語言無關（`use-auth.ts:5` 的同類註解在 F18 一起處理） |
| F24 | `docs/adr/2026-10-18-go-backend.md` | 保留：本次決策紀錄，內文提到 FastAPI 是歷史脈絡 |
