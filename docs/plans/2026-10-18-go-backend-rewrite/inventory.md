# 舊後端盤點（FastAPI → Go）

快照 commit：`7a4d0fa`（2026-10-18）。舊系統是行為的出發點，不是要保留的契約：2026-10-18 Will 拍板「照業界基準重做、不留技術債」，每一列的去向以基準為準，偏離基準的地方要寫明理由。收尾時「去向」不得有空格，由 `check_inventory.py` 檢查。

產生方式：

```bash
# REST operation
rg -n '@router\.(get|post|patch|delete|put)' backend/src
# 表
rg -n '__tablename__' backend/src
# 設定
rg -n '^    [A-Z_]+: ' backend/src/core/config.py
# 後端以外引用 Python 後端或舊 API 的檔案（消費端；排除 node_modules 與 lockfile）
rg -l -i 'backend|python|uv |pytest|ruff|alembic|diff-cover|fastapi|sqlalchemy|pydantic' \
  -g '!backend/**' -g '!frontend/node_modules/**' -g '!**/pnpm-lock.yaml' -g '!docs/plans/**' --hidden -g '!.git/**'
```

## A. REST operation（舊 12 個 → 新 11 個）

新 API 一律掛在 `/api` 底下，和前端同源（B16）。路徑依 `docs/rules/api-design.md` 的「URL 不用動詞」規則改成資源導向；舊的 `/auth/login` 等路徑本身就違反這條規則。

| # | 舊 operation | 舊行為 | 去向 |
|---|---|---|---|
| A1 | `GET /health` | `{"status":"ok"}` | 保留為 `GET /health`，只給容器內部存取，不經 Caddy 對外 |
| A2 | `GET /health/ready` | `SELECT 1`；成功回 `{"status":"ready"}`，失敗回 500 | 保留為 `GET /health/ready`；DB 失敗或 shutdown 中回 503（E7、B19） |
| A3 | `POST /auth/register` | JSON `{account,password}`；建 user，發 access JWT 與 refresh cookie；回 200 `{data: UserResponse}` | 改寫為 `POST /api/users`：同一個 transaction 建 user 與 session，設 session cookie；回 **201**、`Location: /api/users/{id}`，body 直接是 `User` |
| A4 | `POST /auth/login` | form-urlencoded `username`/`password`；dummy hash 防 timing；disabled 帳號視同失敗 | 改寫為 `POST /api/sessions`：body 是 JSON `{account,password}`；回 **201**、`Location: /api/sessions/{id}`，body 是 `Session`（D16）；timing 防護保留；節流規則見 B6 |
| A5 | `POST /auth/refresh` | refresh token 輪替 | 刪除：opaque session 沒有 refresh（D4） |
| A6 | `POST /auth/logout` | 撤銷 refresh token、清 cookie；回 `{"detail":"Logged out"}` | 改寫為 `DELETE /api/sessions/current`：刪除該 session 列、清 cookie；回 **204**。另有 `GET /api/sessions/current`（D16） |
| A7 | `GET /auth/me` | cookie 或 Bearer JWT；disabled 帳號回 401 | 改寫為 `GET /api/users/me`：只認 cookie（Bearer 刪除，因為沒有非瀏覽器 client）；disabled 帳號回 401 |
| A8 | `POST /items` | 建 item，owner 是目前使用者 | 改寫為 `POST /api/items`：回 **201** 與 `Location`，body 直接是 `Item` |
| A9 | `GET /items` | `page`/`size` 分頁；回 `{data,total,page,size,pages}`；沒有 ORDER BY | 改寫為 `GET /api/items?limit=&cursor=`：keyset 分頁，`ORDER BY id DESC`（UUIDv7 依時間排序）；回 `{items, next_cursor}`，不算總數（B9） |
| A10 | `GET /items/{id}` | 任何登入者都能讀任何 item | 改寫為 `GET /api/items/{id}`：非 owner 回 404（E1） |
| A11 | `PATCH /items/{id}`、`DELETE /items/{id}` | 非 owner 回 403；DELETE 回 `{"detail":"Item deleted"}` | 改寫：PATCH 吃 `application/merge-patch+json`（B14）；DELETE 回 **204**；非 owner 一律回 404 |
| A12 | — | 舊系統的 role 沒有任何地方檢查 | 新增 `PATCH /api/users/{id}`（只限 admin，merge-patch，目前只開放 `is_disabled`）；新增 `DELETE /api/sessions`（登出目前使用者的所有 session，回 204）（B17、B18）。D16 追加：`GET /api/users/{id}`（本人或 admin）、`GET /api/users`（admin，keyset）、`GET /api/sessions`、`GET` 與 `DELETE /api/sessions/{id}`（只限自己） |

開發模式的 `/docs`、`/redoc`、`/openapi.json` 改寫為：`MODE=dev` 時提供 `GET /api/openapi.yaml`（契約檔原樣輸出）與 Swagger UI 靜態頁。

## B. 跨切面行為

| # | 舊行為 | 位置 | 去向 |
|---|---|---|---|
| B1 | 回應外殼：`{data}`、`{data,total,page,size,pages}`、`{detail}` | `core/schemas/base.py` | 刪除。單一資源直接回 JSON object；集合回 `{<資源複數>: [...], next_cursor}`；無 body 的操作回 204（RFC 9110；Microsoft／Google API 指南） |
| B2 | 錯誤 `{"detail": str}`；驗證錯誤 422 `{"detail","errors":{field:[msg]}}`；未處理例外 500 | `core/exception_handlers.py` | 改寫成 RFC 9457 `application/problem+json`：<br>• `type` 用固定的相對 URI `/problems/<slug>`，例如 `validation`、`not-found`、`unauthenticated`、`forbidden`、`conflict`、`rate-limited`、`internal`<br>• 另有 `title`、`status`、`detail`<br>• 驗證錯誤帶 `errors: [{pointer, code, detail}]`，`pointer` 是 JSON Pointer，`code` 是穩定的機器碼<br>• 前端依 `type` 與 `code` 走 next-intl 翻譯，不解析 `detail` |
| B3 | `AppException` 家族（404/409/401/403） | `core/exceptions.py` | 改寫：`internal/platform/apperr` 定義 kind 與 code，只在 `internal/httpapi` 一處轉成 problem |
| B4 | middleware：`X-Request-ID`（直接採用 client 送的值）、`X-Process-Time`、五個安全標頭、CORS | `core/middleware.py`、`main.py` | 改寫：<br>• request id：只信任 `TRUSTED_PROXIES` 送來的 `X-Request-ID`，其餘一律重新產生（E5）<br>• 安全標頭：`X-Content-Type-Options: nosniff`、`Cache-Control: no-store`（所有 `/api` 回應）、`Referrer-Policy: no-referrer`；`X-Frame-Options`、`Permissions-Policy`、`X-XSS-Protection` 不放在 JSON API 上（OWASP HTTP Headers：對 JSON 回應沒有作用），HSTS 交給 Caddy<br>• CORS：刪除，因為同源（B16）<br>• `X-Process-Time`：刪除，改記進 access log 的 `duration_ms` |
| B5 | structlog：dev 輸出 console 格式，prod 輸出 JSON，綁定 request_id | `core/logging.py` | 改寫：`log/slog`，dev 用 text、prod 用 JSON；request_id 經 context 帶入 |
| B6 | rate limit：slowapi in-memory，以連線 IP 為 key；4 個 auth 寫入 endpoint 各每分鐘 5 次 | `auth/router.py` | 改寫成兩層：<br>• **依帳號節流**：`login_throttles` 表，key 是正規化後的帳號名，不論帳號是否存在都記錄，避免洩漏帳號存在與否；連續失敗 5 次後，下次允許的時間為 2^(n−5) 分鐘，上限 1 小時；成功登入時歸零（OWASP Authentication、Credential Stuffing Cheat Sheet；NIST 800-63B-4 每個 authenticator 最多 100 次連續失敗）<br>• **per-IP**：`x/time/rate` in-memory，只套用在 `POST /api/users` 與 `POST /api/sessions`；IP 依 `TRUSTED_PROXIES` 解析（E3）。compose 的 `TRUSTED_PROXIES` 設為 compose network 的固定 subnet（`172.30.0.0/24`）：backend 不對外發佈 port，同一 subnet 裡只有 Caddy 與 Next 會連進來<br>• 超限一律回 429 problem 加 `Retry-After`；登出不限流 |
| B7 | 每個請求一個 DB session：成功 commit、例外 rollback | `core/database.py` | 改寫：pgxpool；需要多個語句原子完成的 service 方法自己開 transaction；每條連線以 `RuntimeParams` 設定 `statement_timeout`（`DB_STATEMENT_TIMEOUT`）、`lock_timeout` 3s、`idle_in_transaction_session_timeout` 30s（各自獨立；設計審查 2026-10-18）、`TimeZone=UTC`。migrate 連線的 `lock_timeout` 在步驟 2 與 squawk 一起處理 |
| B8 | `BaseRepository[T]` 泛型 CRUD | `core/repository/base.py` | 刪除：sqlc 為每個查詢生成函式 |
| B9 | `PaginationParams`：page ≥ 1、size 1–100 | `core/dependencies.py` | 改寫成 keyset 分頁：`limit` 1–100（預設 20）、`cursor` 為 opaque 字串（base64url 編碼最後一筆的 id）；不支援 total。理由：Zalando #160、#254，Google AIP-158 |
| B10 | argon2（pwdlib recommended）；無法解析的 hash 視為驗證失敗 | `core/security.py` | 保留：argon2id，m=19 MiB、t=2、p=1（OWASP Password Storage），PHC 字串格式；同時進行的雜湊數以 semaphore 限制（預設 `NumCPU`），避免記憶體被打爆 |
| B11 | `BackgroundTasks` 示範 | `items/router.py` | 刪除：只是示範寫 log，範本不示範背景工作 |
| B12 | 密碼 8–100 字，強制含大寫、小寫、數字；account 3–50 字 | `auth/schemas.py` | 改寫成 NIST 800-63B-4：<br>• 密碼：最少 15 個 code point（只有密碼一個因子時）、最多 128；**不強制字元組合**；比對內嵌的常見與外洩密碼清單（B20），命中回 422 code `password_too_common`<br>• account：建立時契約 pattern `^[A-Za-z0-9_.-]{3,50}$`（接受大小寫），service 存入前轉小寫，所以唯一性不分大小寫；登入時契約只限 1–50 字、不套 pattern，格式不符一律走 401，回應與密碼錯誤相同<br>• 密碼：service 在長度檢查與雜湊之前先做 NFKC 正規化（NIST「SHOULD apply normalization」；長度由 validator 依原始輸入的 code point 計算）<br>• 長度與字元集只在 `openapi.yaml` 寫一次。HTTP 由 request validator 執行；`seed-admin` 從內嵌的 `openapi.yaml` 取 `CreateUserRequest` schema，以 kin-openapi 驗證輸入；清單比對與唯一性由 service 檢查<br>• 前端不重複長度與 pattern 檢查，只做「必填」的即時提示，其餘依後端 problem 的 `pointer` 與 `code` 顯示在對應欄位 |
| B13 | `UserResponse {id:int, account, is_disabled, role:int}`；`ItemResponse {id:int, title, description\|null, owner_id:int, created_at, updated_at}` | schemas | 改寫：<br>• `User {id: uuid 字串, account, role: "admin"\|"user", is_disabled, created_at}`<br>• `Item {id: uuid, title, description\|null, owner_id: uuid, created_at, updated_at}`<br>• 時間一律 RFC 3339 UTC |
| B14 | `PATCH /items/{id}` 三態：沒送的欄位不改，`description: null` 清成 NULL | `items/service.py` `update_item` | 保留語意，改為正式宣告 JSON Merge Patch（RFC 7396，`application/merge-patch+json`）；request 型別用 `nullable.Nullable[T]`；`title: null` 回 422（E9） |
| B15 | disabled 帳號：登入 401（訊息與密碼錯誤相同）、`/auth/me` 401 | `auth/service.py:50`、`auth/dependencies.py` | 保留；改成被停用時同時刪除該使用者的所有 session（B18）。舊系統沒有測試，這次補上 |
| B16 | 前端在 `yourdomain.com`、API 在 `api.yourdomain.com`；CORS；build 時寫死 `NEXT_PUBLIC_API_URL`；CSP 的 `connect-src` 帶 API 網址 | `docs/deploy-vps.md:61,109-113`、`frontend/src/proxy.ts`、`.env-example` | 改寫成同源：<br>• 正式環境：compose 裡的 `caddy` service（`deploy/Caddyfile`）以 `handle /api/*` 轉給 backend，其餘給 Next；只有 Caddy 發佈 80/443，backend 與 frontend 不發佈 port；站點位址由 `SITE_ADDRESS` 設定（本機 `make up` 用 `:80`）<br>• dev（`next dev`）：`next.config.ts` 的 `rewrites` 只在 `NODE_ENV=development` 生效，把 `/api/:path*` 轉到 `API_URL`；dev server 啟動時讀 runtime 環境變數，standalone build 不依賴 rewrites<br>• 瀏覽器一律打相對路徑 `/api`；Next 的 server-side fetch 用 `API_URL`（容器內網址）並轉發 cookie<br>• CSP 改為 `connect-src 'self'`；`NEXT_PUBLIC_API_URL` 刪除 |
| B17 | role：int 1/2/3，註冊預設 GUEST（3），全專案沒有任何地方檢查 role | `auth/constants.py`、`auth/models.py` | 改寫：`role text CHECK (role IN ('admin','user'))`，預設 `user`；admin 只能透過 `seed-admin` 建立（帳號已存在時不做任何修改，以 exit 0 結束並印出提示）；`PATCH /api/users/{id}` 是唯一需要 admin 的操作，並寫成測試，示範授權模型 |
| B18 | session 撤銷只有登出 | — | 新增：停用帳號時刪除該使用者的所有 session；新增 `DELETE /api/sessions`（登出所有裝置）。範本沒有改 role、改密碼的功能；規則「改 role、改密碼時撤銷該使用者所有 session」寫進 `docs/rules/api-design.md`，供之後新增時遵守 |
| B19 | uvicorn 預設行為：沒有 request body 上限，timeout 交給 uvicorn | `Dockerfile` CMD | 改寫：`http.Server` 設定 `ReadHeaderTimeout` 5s、`ReadTimeout` 15s、`WriteTimeout` 30s、`IdleTimeout` 120s；每個 body 用 `http.MaxBytesReader` 限制在 1 MiB；收到 SIGTERM 後直接 `Shutdown`（上限 30s，不小於 `WriteTimeout`），最後關閉 pool；compose `stop_grace_period` 35s。不做 readiness drain（D14：單一 backend 沒有 LB 依 readiness 摘流量）|
| B20 | — | — | 新增：內嵌常見與外洩密碼清單，存成 `internal/modules/auth/blocklist/*.txt`（`go:embed`），來源與授權在步驟 0 核准；以設定 `PASSWORD_HIBP_CHECK=true` 可另外查 HIBP k-anonymity API（預設關閉） |
| B21 | 沒有 CSRF 防護，只靠 `SameSite=Lax`（`docs/frontend-known-issues.md:18`） | — | 新增：在 mux 外層套 Go 1.25 的 `http.CrossOriginProtection`（看 `Sec-Fetch-Site`，沒有時退回 `Origin`）；`docs/frontend-known-issues.md` 那段改寫（E12） |
| B22 | image 的 `HEALTHCHECK` 打 `/health`（`python -c urllib`） | `backend/Dockerfile` | 改寫（步驟 1，設計審查補列）：distroless 沒有 shell 與 curl，改由 `app healthcheck` 子指令打 `/health/ready`；compose 的 backend `healthcheck` 用它，frontend 以 `service_healthy` 等 backend，CI 的 stack job 與 `reset-db` 用 `docker compose up --wait` |

## C. 表（舊 3 張 → 新 4 張）與 migration（4 支）

型別規則照 D6（PG wiki「Don't Do This」）：主鍵 `uuid DEFAULT uuidv7()`；文字用 `text`，有長度限制時加 CHECK；狀態欄用 `text` 加 CHECK；時間一律 `timestamptz`；`updated_at` 由 sqlc 查詢明寫 `= now()`，並有測試覆蓋。

| # | 舊 | 去向 |
|---|---|---|
| C1 | `users(id serial, account varchar(50) unique, password varchar(255), is_disabled bool, role int default 3, created_at, updated_at)`；`id`、`role` 都是 int4，見 `alembic/versions/2024_05_16_0041-*.py:26,30` | 改寫為 `users`：<br>• `id uuid PK DEFAULT uuidv7()`<br>• `account text NOT NULL UNIQUE CHECK (account = lower(account) AND char_length(account) BETWEEN 3 AND 50)`<br>• `password_hash text NOT NULL`<br>• `role text NOT NULL DEFAULT 'user' CHECK (role IN ('admin','user'))`<br>• `is_disabled boolean NOT NULL DEFAULT false`<br>• `created_at`、`updated_at timestamptz NOT NULL DEFAULT now()` |
| C2 | `refresh_tokens(token_hash, user_id, expires_at, revoked, created_at)` | 刪除，改為 `sessions`：<br>• `token_hash bytea PK`（SHA-256）<br>• `id uuid NOT NULL DEFAULT uuidv7()`，UNIQUE：API 用的公開 id，token 的衍生值不外露（D16）<br>• `user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE`，索引 `(user_id, id DESC)` 供列表 keyset 與整批刪除<br>• `created_at`、`last_used_at`、`expires_at timestamptz NOT NULL` |
| C3 | `items(id, title varchar(200), description text null, owner_id fk, created_at, updated_at)`；`owner_id` 有 index | 改寫為 `items`：<br>• `id uuid PK DEFAULT uuidv7()`<br>• `title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200)`<br>• `description text`，可為 NULL<br>• `owner_id uuid NOT NULL REFERENCES users`，不 cascade<br>• 複合索引 `(owner_id, id DESC)`，供 keyset 分頁使用<br>• `owner_id` 是唯一允許的跨模組 FK（計畫 I4） |
| C4 | — | 新增 `login_throttles`：<br>• `account_key text PK`<br>• `failures bigint NOT NULL DEFAULT 0 CHECK (failures >= 0)`（squawk 的 `prefer-bigint-over-int`；步驟 3a 由 int 改）<br>• `next_allowed_at timestamptz`<br>• `updated_at timestamptz NOT NULL`<br>屬於 auth 模組；成功登入時刪除該列 |
| C5 | 4 支 Alembic migration | 刪除。範本沒有部署、沒有資料（計畫 P1），改由 Atlas 產生 baseline migration |

## D. 設定（環境變數）

| # | 舊 | 去向 |
|---|---|---|
| D1 | `APP_NAME`、`APP_VERSION`、`MODE`（dev/prod） | `APP_NAME`、`MODE` 保留；`APP_VERSION` 刪除，改在 build 時以 `-ldflags` 注入 |
| D2 | `DATABASE_URL`，以 `postgresql+asyncpg://` 開頭，程式會自動轉換 | 保留變數名，接受 `postgres://` 與 `postgresql://`；帶 `+asyncpg` 之類的 driver 後綴時，啟動直接失敗並提示改法 |
| D3 | `SECRET_KEY`（至少 32 字，prod 擋 `change-me`）、`ALGORITHM`、`ACCESS_TOKEN_EXPIRE_MINUTES`、`REFRESH_TOKEN_EXPIRE_DAYS` | 刪除：opaque session 不需要簽章。改成 `SESSION_ABSOLUTE`（預設 `168h`）與 `SESSION_IDLE`（預設 `24h`），且 idle 不得大於 absolute；兩者都在 NIST AAL1 的 30 日上限內。`scripts/init_env.py` 拿掉產生 SECRET_KEY 的那段 |
| D4 | `COOKIE_SECURE`、`COOKIE_SAMESITE`（含值驗證；none 必須搭配 Secure）、`COOKIE_DOMAIN` | `COOKIE_SECURE` 保留（預設 true），只為了本機 http 開發；`COOKIE_SAMESITE`、`COOKIE_DOMAIN` 刪除：同源部署下固定 `SameSite=Lax`、不帶 Domain。cookie 名稱：Secure 時為 `__Host-session`，非 Secure 時為 `session`（`__Host-` 前綴必須搭配 Secure） |
| D5 | `DB_POOL_SIZE`、`DB_MAX_OVERFLOW`、`DB_POOL_RECYCLE` | 改為 `DB_MAX_CONNS`（預設 20）與 `DB_STATEMENT_TIMEOUT`（預設 `5s`），其餘刪除 |
| D6 | `CORS_ORIGINS` | 刪除：同源部署（B16） |
| D7 | — | 新增：<br>• `TRUSTED_PROXIES`：CIDR 清單，預設空，表示不信任 XFF 與 `X-Request-ID`<br>• `AUTH_RATE_LIMIT_PER_MIN`：預設 5。數字沿用舊值，沒有業界標準值（OWASP Credential Stuffing 只要求分層）；暴力破解已由依帳號節流處理，這層只擋大量註冊與噴灑。重新評估：共用 NAT 的正常使用者回報 429<br>• `PASSWORD_HIBP_CHECK`：預設 false<br>• `ADMIN_ACCOUNT`、`ADMIN_PASSWORD`：seed 用<br>• frontend 的 `API_URL`：容器內的後端網址 |
| D8 | 所有 service 共用 `env_file: .env`；app 與 migrate 都用 `POSTGRES_USER`（superuser），密碼寫在 `DATABASE_URL` | 改寫（D15，步驟 2）：`<app>_owner`（migrate）與 `<app>_app`（serve、seed-admin）兩個角色；密碼以 compose secrets 掛檔，`POSTGRES_PASSWORD_FILE`、`DATABASE_PASSWORD_FILE`；`DATABASE_URL` 不含密碼；每個 service 只拿自己的變數 |

## E. 既有缺陷

| # | 缺陷 | 證據 | 處置 |
|---|---|---|---|
| E1 | `GET /items/{id}` 沒有檢查 owner，任何登入者都能讀別人的 item（IDOR） | `items/service.py` 的 `get_item` 不帶 owner 條件 | 修：非 owner 回 404。回歸測試：使用者 2 讀使用者 1 的 item 要回 404 |
| E2 | access token 還有效但 user 已被刪除時，`/auth/me` 回 404 而不是 401 | `auth/service.py:get_user_by_account` | 修：sessions 有 CASCADE，user 刪除後 session 查不到即回 401。回歸測試 |
| E3 | rate limit 以連線 IP 為 key；走 Caddy 時全站共用同一個 bucket | slowapi `get_remote_address`；`docs/deploy-vps.md` §5 | 修：B6 兩層節流。回歸測試：兩個不同 XFF 各自計數；不受信任來源送的 XFF 被忽略 |
| E4 | `X-XSS-Protection: 1; mode=block` 會引入 XS-Leak | `core/middleware.py` | 修：JSON API 不再送這個標頭（B4）。測試斷言回應沒有此標頭 |
| E5 | 直接採用 client 送的 `X-Request-ID`，可以把任意字串注入 log | `core/middleware.py` | 修：只信任 proxy 送的值，且限 ≤64 字的 `[A-Za-z0-9-]`。回歸測試 |
| E6 | `GET /items` 分頁沒有 ORDER BY，跨頁結果不穩定 | `items/repository.py` | 修：keyset 分頁加 `ORDER BY id DESC`（B9）。回歸測試：分頁期間新增資料，不重複也不漏 |
| E7 | `/health/ready` DB 失敗時回 500 | `health/router.py` | 修：回 503。測試 |
| E8 | `make seed` 建出固定帳密 `admin`／`Admin123` | `Makefile:72-88` | 修：`app seed-admin` 讀 `ADMIN_ACCOUNT`、`ADMIN_PASSWORD`，沒設就失敗；密碼套用 B12 規則（以內嵌 spec 驗證）；帳號已存在時不修改、exit 0 |
| E9 | `ItemCreate.title` 沒有長度限制、`ItemUpdate.title` 可以送 null；超長或 null 寫入 NOT NULL 欄位時回 500 | `items/schemas.py` 對照 `String(200)` | 修：title 必填、1–200 字，契約與 DB CHECK 兩層都檢查；PATCH 送 `title: null` 回 422。回歸測試 |
| E10 | `test_config.py` 的 5 條設定驗證：3 條 SameSite、2 條 `SECRET_KEY` | `tests/test_config.py:16-35` | 這些設定已隨 D3、D4 刪除，改測新設定：`SESSION_IDLE` > `SESSION_ABSOLUTE` 失敗、`DATABASE_URL` 帶 `+asyncpg` 失敗、`TRUSTED_PROXIES` 格式錯誤失敗 |
| E11 | 部署成前端與 API 分開兩個子網域時，cookie 沒帶 Domain，只屬於 `api.` 這個 host，前端的 `proxy.ts` 與 `getServerUser()` 收不到，伺服器端的登入檢查會一直判定沒登入。這是依 cookie 規範推論，沒有實測 | `docs/deploy-vps.md:61,112`、`auth/cookies.py`（`COOKIE_DOMAIN` 預設 None） | 修：改成同源（B16）。回歸驗收：步驟 5 的端到端測試走 Caddy 同源設定 |
| E12 | 沒有 CSRF 防護；`api.` 子網域與主網域屬於同一個 site，SameSite 擋不到從兄弟子網域發起的請求 | `docs/frontend-known-issues.md:18-19` | 修：B21。回歸測試：帶 `Sec-Fetch-Site: cross-site` 的 `POST /api/items` 回 403 |
| E13 | 註冊預設 role 是 GUEST，但全專案沒有任何地方檢查 role | `auth/constants.py`；`rg -n role backend/src frontend/src` 只命中 model、schema 與 types | 修：B17 |
| E14 | 密碼規則違反 NIST 800-63B-4：最少只要 8 字、強制字元組合、沒有清單比對 | `auth/schemas.py` | 修：B12、B20 |
| E15 | compose 的 port 綁在 `0.0.0.0`（`"8000:8000"`、`"3000:3000"`）；Docker 發佈的 port 會繞過 ufw（未查證） | `docker-compose.yml` | 修：正式環境的 compose 只有 `caddy` 發佈 80/443，backend 與 frontend 不發佈 port；dev compose 的 port 綁 `127.0.0.1` |

## F. 消費端

搜尋指令見檔頭第四條；2026-10-18 命中 31 個檔案，含本次新增的 ADR，`.claude/skills/*` 也在內。`backend/` 內的 `AGENTS.md`、`README.md`、`Makefile`、`Dockerfile` 不在搜尋範圍內，另列在 F8 與步驟 1。

| # | 檔案 | 去向 |
|---|---|---|
| F1 | `.github/workflows/ci.yml` | 改寫：Go job 跑 golangci-lint、`go test -race`、coverage → Cobertura → diff-cover、govulncheck、sqlc diff、Atlas schema 漂移與 squawk、oasdiff；frontend job 加上「從 `openapi.yaml` 重新生成的型別必須和 commit 的一致」 |
| F2 | `.github/dependabot.yml`（`uv`） | 改寫：加 backend 的 `gomod` 與 `docker`；刪除 `uv` |
| F3 | `lefthook.yml` | 改寫：pre-commit 對有改動的檔案跑 `gofmt` 與 golangci-lint |
| F4 | `Makefile`（root） | 改寫：`migration`、`generate-migration` 改用 Atlas（步驟 1）；`seed`、`reset-db`（後者呼叫 `wait_for_db.py`）改成 `app seed-admin` 加輪詢 `/health/ready`（步驟 3）；`dev` 改成 `docker compose ... watch`（P9，步驟 1）；其餘保留 |
| F5 | `docker-compose.yml`、`docker-compose.dev.yml` | 改寫：migrate service 跑 `atlas migrate apply`；network 固定 subnet `172.30.0.0/24`，Caddy 固定 IP，`TRUSTED_PROXIES` 只列 Caddy（D18）；加 `caddy` service，只有它發佈 port（E15，步驟 5），dev 也走 Caddy，port 綁 `127.0.0.1`（D18）；frontend 設 `API_URL`；dev 用 `docker compose watch` 的 `rebuild`（計畫 P9） |
| F6 | `.env-example`、`frontend/.env.example` | 改寫：依 D 段；刪除 `NEXT_PUBLIC_API_URL` |
| F7 | `AGENTS.md`（root） | 改寫：Verification（`make test` → Go）、Conventions、Structure |
| F8 | `backend/AGENTS.md`、`backend/README.md`、`backend/Makefile`、`backend/Dockerfile` | 改寫成 Go 版：`Makefile`、`Dockerfile` 在步驟 1，文件在步驟 6 |
| F9 | `docs/rules/api-design.md`、`docs/rules/database-design.md` | 改寫成 B、C 段的基準：資源導向 URL、不用外殼、201/204、keyset 分頁、merge-patch、RFC 9457 加 code、`text` 加 CHECK、UUIDv7、session 撤銷規則 |
| F10 | `docs/deploy-vps.md` | 改寫：單一網域，Caddy 跑在 compose 內（`deploy/Caddyfile`），`/api/*` 轉給 backend；HSTS 設在 Caddy；刪除 host 上安裝 Caddy／Nginx 的段落、`NEXT_PUBLIC_API_URL` 與 `api.` 子網域；改用外部 proxy 時要設定 `TRUSTED_PROXIES` 的說明 |
| F11 | `README.md` | 改寫 |
| F12 | `scripts/rename.py` | 改寫：`go mod edit -module`、改寫所有 import 前綴、用 `go build ./...` 驗證；刪除 `pyproject.toml` 那一列；`brand_files` 的 `CLAUDE.md` 改成 `AGENTS.md` |
| F13 | `scripts/init_env.py` | 改寫：依 D3 |
| F14 | `.claude/skills/add-module.md`、`.claude/skills/add-crud.md` | 改寫成 Go 模組骨架：`schema.sql`、queries、sqlc、`httpapi` handler、OpenAPI 片段、前端型別重新生成 |
| F15 | `.gitignore` | 改寫：刪除 Python 相關項目，加入 Go 產物 |
| F16 | `frontend/src/lib/api-client.ts`、`frontend/src/lib/__tests__/api-client.test.ts` | 改寫：改用 openapi-fetch 搭配生成的 `paths` 型別，baseUrl 為 `/api`；刪除 401 自動 refresh 與 mutex；`ApiError` 改成帶 problem 的 `type`、`errors[]`（含 `pointer` 與 `code`）。步驟 5 落在 `frontend/src/lib/api/client.ts`、`frontend/src/lib/api/errors.ts`、`frontend/src/lib/api/schema.d.ts`（由 `frontend/package.json` 的 `gen:api` 生成，`gen:api:check` 在 CI 檢查） |
| F17 | `frontend/src/proxy.ts` | 改寫：判斷 `__Host-session` 或 `session` cookie 是否存在；刪除 refresh 轉發；刪除「有 cookie 就把 login 頁導去 overview」（cookie 可能已失效，會和 dashboard 的 `getServerUser()` 互相導向）；CSP 改為 `connect-src 'self'` |
| F18 | `frontend/src/features/auth/api/auth-api.ts`、`frontend/src/features/auth/hooks/use-auth.ts` | 改寫：改呼叫新路徑，login 送 JSON；刪除 `refresh`；回應直接是 `User`（沒有 `.data`）；註解更新 |
| F19 | `frontend/src/features/auth/server.ts`、`frontend/src/features/auth/__tests__/server.test.ts` | 改寫：改打 `GET /api/users/me`，回應沒有 `.data` |
| F20 | `frontend/src/features/auth/types.ts` | 刪除，改用生成的型別（`components["schemas"]["User"]`） |
| F21 | `frontend/src/app/layout.tsx:10,14,22`、`frontend/messages/en.json:19`、`frontend/messages/zh-TW.json:19` | 改寫：描述中的「FastAPI」改成「Go」，「k3s」改成 Docker Compose；messages 加上各 problem `type` 與驗證 `code` 的翻譯 |
| F22 | `frontend/AGENTS.md:14,55,65-78` | 改寫：auth 流程改成新路徑、同源、單一 session cookie、沒有 refresh |
| F23 | `frontend/README.md:14`、`docs/frontend-known-issues.md:15,17` | `README.md:14` 改寫：刪除 `NEXT_PUBLIC_API_URL` 的說明；`frontend-known-issues.md` 改寫 CSRF 段（E12） |
| F24 | `docs/adr/2026-10-18-go-backend.md`、`docs/adr/2026-10-18-api-and-auth-baseline.md` | 保留：本次的決策紀錄，內文提到 FastAPI 是歷史脈絡 |
| F25 | `frontend/next.config.ts` | 改寫：加 `rewrites` 把 `/api/:path*` 轉到 `API_URL`，只在 development 生效，只給 host 上的 `pnpm dev` 用（compose 的 dev 走 Caddy，D18）；安全標頭保留（這些是 HTML 頁面用的） |
| F26 | `frontend/src/features/auth/validations.ts`、`frontend/src/features/auth/components/login-form.tsx` | 改寫：zod 只保留「必填」檢查，刪除長度與組合規則（避免在前端重複契約，見 B12）；錯誤依 problem 的 `pointer` 顯示在對應欄位、依 `code` 翻譯，不顯示 `detail` |
| F27 | `frontend/Dockerfile`（`ARG NEXT_PUBLIC_API_URL`） | 改寫：刪除該 build arg（B16） |
| F28 | `AGENTS.local.md`（git 不追蹤）、專案頁 frontmatter `tech`／`tags`、Architecture、Key Files | 改寫（步驟 8）：計畫刪除後移除指向；專案頁改成 Go 版描述 |
| F29 | `deploy/postgres/initdb/10-roles.sh` | 保留：步驟 2a 新增的 initdb 腳本（D15）；命中搜尋是因為註解引用 `docs/adr/2026-10-18-go-backend.md` |
