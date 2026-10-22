# 威脅建模：認證與 session

用 STRIDE 逐項檢查帳號、密碼、session 與角色在目前程式碼裡的做法。每個緩解措施都寫出程式碼或證明它的測試；找不到程式碼的威脅列為 Open。設計的決定在 [API 與認證基準](../adr/2026-10-18-api-and-auth-baseline.md)（D4 session、D5 CSRF、D6 密碼與節流、D7 授權）與 [Go 後端](../adr/2026-10-18-go-backend.md)（D5 資料庫角色）。

路徑相對於 repo 根目錄；`httpapi/` 指 `backend/internal/httpapi/`，`auth/` 指 `backend/internal/modules/auth/`。

## 範圍

**資產**

| 資產 | 存放位置 |
|---|---|
| 帳號 | `users` 表：`account`（小寫、唯一）、`role`、`is_disabled`（`auth/schema.sql`） |
| 密碼雜湊 | `users.password_hash`，argon2id PHC 字串（`auth/password.go`） |
| Session token | cookie 裡的 32 個隨機 byte；資料庫只存它的 SHA-256（`sessions.token_hash`，`auth/sessions.go`） |
| 角色 | `users.role`，`admin` 或 `user`；admin 只能由 `app seed-admin` 建立（`backend/cmd/app/seed.go`） |
| 節流狀態 | `sign_in_throttles`（`auth/schema.sql`） |

**信任邊界**

1. 瀏覽器 → Caddy：只有 Caddy 發佈 port（`docker-compose.yml`）。`SITE_ADDRESS` 是網域時有 TLS 與 HSTS（`deploy/Caddyfile`）。
2. Caddy → backend（`/api/*`）與 Caddy → Next（其餘），走 compose 網路。backend 只相信 `TRUSTED_PROXIES`（預設是 Caddy 的固定位址）送來的 `X-Forwarded-For` 與 `X-Request-ID`。
3. Next server → backend：server 端呼叫直接打 `API_URL`，只轉送請求的 cookie（`frontend/src/features/auth/server.ts`）。Next 不是受信任的 proxy，無法指定 client 位址。
4. backend → PostgreSQL：用 app 角色，只有 DML 權限（`deploy/postgres/initdb/10-roles.sh`、`backend/internal/platform/migrate/migrate.go`）。
5. backend → Have I Been Pwned range API：只在 `PASSWORD_HIBP_CHECK=true` 時（`backend/internal/platform/hibp/hibp.go`）。

**入口**（`backend/api/openapi.yaml` 的 operation；誰能呼叫由 `httpapi/authz.go` 的 `operationAccess` 決定，啟動時與契約比對）

| operationId | Route | 誰能呼叫 |
|---|---|---|
| `createUser` | `POST /api/users` | 匿名；每個 IP 限流 |
| `createSession` | `POST /api/sessions` | 匿名；每個 IP 限流，每個帳號節流 |
| `deleteCurrentSession` | `DELETE /api/sessions/current` | 匿名也可，讓過期的 cookie 也能登出 |
| `getCurrentUser`、`getUser` | `GET /api/users/me`、`GET /api/users/{id}` | 登入者；`getUser` 只能看自己，admin 例外 |
| `listSessions`、`getSession`、`deleteSession`、`deleteAllSessions`、`getCurrentSession` | `/api/sessions…` | 登入者，只限自己的 session |
| `deleteCurrentUser` | `DELETE /api/users/me` | 登入者；session 必須是 10 分鐘內登入的，否則 403 `reauthentication_required` |
| `listUsers`、`patchUser`、`deleteUser` | `GET /api/users`、`PATCH`／`DELETE /api/users/{id}` | 只有 admin |

不在範圍內：`items` 的各個 operation（擁有者範圍之外）、主機與 Docker 的強化、Caddy 的 TLS 設定。

## Spoofing（冒充）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 對單一帳號線上猜密碼 | 每個「帳號＋來源」（位址，IPv6 以 /64；或信任的裝置）各自節流，在比對密碼之前計數，第 5 次失敗起等待時間加倍、上限 1 小時；沒有該帳號裝置 cookie 的來源另外合計，連續失敗 100 次（NIST SP 800-63B 3.2.2）起全部拒絕，直到登入成功、admin 解鎖或 24 小時沒有嘗試（`auth/devices.go`、`auth/queries/sign_in_throttles.sql`）。測試：`TestSignInThrottlePerClient`、`TestUntrustedClientsLockedTrustedDeviceNot`、`TestLoginThrottlePerAccount`、`TestLoginThrottleConcurrent`、`TestLoginThrottleSchedule` | 已緩解 |
| 從同一位址對多個帳號噴灑密碼 | `CreateUser`、`CreateSession` 的每 IP token bucket，預設每分鐘 5 次，IPv6 以 /64 為鍵（`httpapi/ratelimit.go`）。測試：`TestRateLimitAuthOperations`、`TestIPLimiterKeysIPv6By64` | 單一副本內已緩解；見「已接受的風險」 |
| 弱密碼或外洩過的密碼 | 15 到 128 字元（契約）、NFKC 正規化、內建常見密碼清單、可選的 HIBP 檢查（`auth/auth.go`、`auth/password.go`）。測試：`TestCreateUserPasswordTooShort`、`TestCreateUserCommonPassword`、`TestCreateUserBreachedPassword`、`TestPasswordNormalized` | 已緩解。HIBP 預設關閉，查詢失敗時放行：接受，外部服務故障不該擋住註冊 |
| 猜 session token | `crypto/rand` 產生的 256-bit token（`auth/sessions.go`）。測試：`TestCreateUserSetsSessionCookie` | 已緩解 |
| Session fixation | 每次登入都發新 token，並刪掉請求帶來的舊 token。測試：`TestCreateSessionIssuesNewToken` | 已緩解 |
| 登出、停用或過期後重用 token | 每個請求都重讀 session 與使用者；idle 預設 1 天、絕對期限 7 天；停用會刪掉該使用者所有 session。測試：`TestSessionIdleExpiry`、`TestSessionAbsoluteExpiry`、`TestDeleteCurrentSessionRevokes`、`TestDeleteAllSessions`、`TestAdminDisablesUserRevokesSessions`、`TestMeDisabledAccount`、`TestMeAfterUserDeleted` | 已緩解 |
| 用 header 代替 cookie 送 token | 只讀 cookie（`httpapi/auth.go`）。測試：`TestMeIgnoresBearer` | 已緩解 |
| Client 偽造位址規避每 IP 限流 | `X-Forwarded-For` 從右往左走，只經過受信任的 proxy（`httpapi/middleware.go`）。測試：`TestClientIP`、`TestRateLimitIgnoresXFFFromUntrustedPeer`、`TestRateLimitPerIPBehindTrustedProxy` | `TRUSTED_PROXIES` 只列 proxy 時已緩解 |
| 被停用的帳號登入 | 回應與密碼錯誤相同的 `invalid_credentials`。測試：`TestCreateSessionDisabledAccount` | 已緩解 |

## Tampering（竄改）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 跨站請求偽造（CSRF） | `http.CrossOriginProtection` 包住整個 mux；cookie 是 `SameSite=Lax`（`httpapi/server.go`、`httpapi/auth.go`）。測試：`TestCrossOriginPostRejected`、`TestCreateItemCrossSite` | 已緩解。`Sec-Fetch-Site` 與 `Origin` 都沒帶的請求會放行；瀏覽器在會改變狀態的請求上都會帶 |
| 註冊時自己指定角色 | 請求 schema 拒絕未定義的欄位；service 一律寫入 `user`。測試：`TestValidationCodes`（`additional_property`）、`TestCreateUserSetsSessionCookie`（角色是 `user`） | 已緩解 |
| 同一帳號名稱同時註冊 | 由唯一索引決定，恰好一個成功，其餘 409。測試：`TestCreateUserConcurrentSameAccount` | 已緩解 |
| Token 走明文 http | 除非 `COOKIE_SECURE=false`，cookie 帶 `Secure` 與 `__Host-` 前綴；Caddy 送 HSTS。測試：`TestCookieNameFollowsSecure`、`TestConfigDefaults` | `SITE_ADDRESS` 是網域時已緩解；`:80` 只供本機使用 |
| 在日誌裡偽造 request id | 只接受受信任來源、格式為 `[A-Za-z0-9-]{1,64}` 的值；Caddy 覆寫 client 送的值。測試：`TestRequestIDFromUntrustedPeerReplaced`、`TestRequestIDMalformedReplaced` | 已緩解 |
| SQL injection | 查詢只經過 sqlc 產生的程式碼；手寫 pgx `Query` 會被 lint 擋（`backend/.golangci.yml` forbidigo） | 已緩解 |
| 登入後被導向外站 | `callbackUrl` 用 URL 解析後必須與本站同源，`/\evil.example`、`/\t/evil.example` 這類看起來像本站路徑的值也會被擋（`frontend/src/features/auth/callback.ts`）。測試：`callback.test.ts` | 已緩解 |

## Repudiation（否認）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 無法追蹤某個請求 | 每個請求一行 access log，帶 `request_id`、`method`、`path`、`status`、`duration_ms`、`client_ip`（`httpapi/middleware.go`）。測試：`TestPanicLogAndAccessLogCarryRequestID` | 已緩解 |
| 使用者或 admin 否認做過某個操作 | admin 的停用、啟用、刪除、解鎖與主機上的 `recover-admin`，在同一個交易寫進 `audit_events`（執行者、動作、對象、request id）；app 角色對它只能新增與讀取，保留 365 天。註冊、登入成功與失敗、登出、被拒絕的 admin 操作寫安全事件日誌，帶 user id 與 request id；容器日誌有輪替。測試：`TestAdminChangesAreAudited`（每個會改狀態的 admin operation 都要有案例）、`TestAuditEventsAppendOnly`、`TestAppendOnlyTables`、`TestSecurityEventsLogged` | 已緩解。紀錄存在同一個資料庫，能拿到 owner 或 superuser 權限的人仍可修改；需要防竄改證明時改送外部儲存 |

## Information disclosure（資訊洩漏）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 資料庫外洩導致 session 被盜用 | 只存 token 的 SHA-256，schema 用 CHECK 限制 32 byte。測試：`TestSchemaChecks` | 已緩解 |
| 資料庫外洩導致密碼外洩 | argon2id m=19 MiB、t=2、p=1、16 byte salt，比對用 constant time（`auth/password.go`）。測試：`TestArgon2Hash` | 已緩解 |
| 密碼或 token 寫進日誌 | 註冊、登入成功與失敗、驗證錯誤、格式錯誤的 body、未知的 token、登出、HIBP 查詢失敗，全部跑一遍後檢查日誌（含 debug 級）。測試：`TestCredentialsNeverLogged` | 已緩解 |
| Script 讀到 token（XSS） | cookie 是 `HttpOnly`；頁面有 nonce 型 CSP，`connect-src 'self'`、`frame-ancestors 'none'`（`frontend/src/proxy.ts`）。測試：`TestCreateUserSetsSessionCookie`、前端 `proxy.test.ts` | build 出來的版本已緩解；`next dev` 不設 CSP |
| 登入時從回應或時間差列舉帳號 | 不存在的帳號也跑一次 dummy hash，回同樣的 problem；節流也套用在不存在的帳號名。測試：`TestCreateSessionUnknownAccountRunsHash`、`TestLoginThrottlePerAccount` | 已緩解 |
| 註冊時列舉帳號 | 帳號已存在回 `409 account_taken`。測試：`TestCreateUserDuplicate` | 接受：用帳號名而不是 email，無法隱藏名稱已被使用（ADR D6）；註冊有每 IP 限流 |
| 讀到別人的使用者資料或 session | 非擁有者一律 404（`auth/auth.go`、`auth/queries/sessions.sql`）。測試：`TestGetOtherUserNotFound`、`TestDeleteOtherUsersSession`、`TestNonAdminCannotListUsers` | 已緩解 |
| 內部錯誤洩漏細節 | 非預期錯誤變成不帶細節的 500，細節只進日誌（`httpapi/problem.go`）。測試：`TestUnhandledErrorHidden` | 已緩解 |
| 密碼送到 HIBP | 只送 SHA-1 的前 5 個 hex 字元，帶 `Add-Padding`（`hibp.go`）。測試：`TestBreached` | 已緩解。HIBP 網路錯誤的訊息含請求網址，也就是同樣的 5 個字元，會寫進本機日誌（推論自 `net/http` 的 `*url.Error`）：接受，內容與送出去的相同 |
| API 契約與 Swagger UI 外露 | 只在 `MODE=dev` 提供，預設 `prod`。測試：`TestDocs` | 已緩解 |
| backend ↔ PostgreSQL 傳輸未加密 | 連線必須 `sslmode=verify-full`；只有設定 `DATABASE_PLAINTEXT=same-host` 才接受不加密，範本只在 compose 裡設定（同一台主機的內部網路，資料不出主機）。改接遠端資料庫卻沒加密時，backend 與 migrate 拒絕啟動（`internal/platform/config`）。測試：`TestDatabaseURLRequiresVerifiedTLS` | 已緩解。compose 內部不加密是接受的取捨：要防同一網路上被入侵的容器，或資料庫搬到另一台主機時，改用 TLS |

## Denial of service（阻斷服務）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 大量雜湊耗盡記憶體或 CPU | 同時最多 `runtime.NumCPU()` 個 argon2id 雜湊，等待者隨 context 結束（`auth/password.go`）。測試：`TestArgonConcurrencyBounded` | 已緩解 |
| 過大或過慢的請求 | body 上限 1 MiB；header、讀、寫逾時（`httpapi/server.go`）。測試：`TestBodyTooLarge`、`TestSlowHeaderClosed` | 已緩解 |
| 故意登入失敗把受害者鎖在門外 | 節流以「帳號＋來源」為單位，攻擊者的失敗只讓自己的來源等待；帳號層級的鎖只擋沒有該帳號裝置 cookie 的來源，本人曾登入過的瀏覽器照常登入（OWASP「Slow Down Online Guessing Attacks with Device Cookies」）；admin 可查看與解鎖（`GET`／`DELETE /api/users/{id}/sign-in-lock`，寫入稽核）。全部登出、停用、救援時忘掉該帳號的裝置。測試：`TestUntrustedClientsLockedTrustedDeviceNot`、`TestDeviceCookieIsPerAccount`、`TestSignOutEverywhereForgetsDevices`、`TestAdminUnlocksSignIn` | 已緩解。本人在新裝置上仍可能被鎖，要等 admin 解鎖；加了 email 之後改成鎖定期間可用重設密碼解鎖（ADR api-and-auth D6 的重新評估條件） |
| 用亂數帳號名塞滿 `sign_in_throttles` | 不存在的帳號名也會建立紀錄（每次嘗試最多兩列：來源一列、帳號合計一列）；每 IP 限流限制速率，閒置 24 小時的紀錄每小時清掉。測試：`TestCleanup` | 單一副本內已緩解 |
| Admin 停用最後一個 admin，包括自己 | 停用時先鎖住所有啟用中的 admin，目標是唯一一個就回 409 `last_admin`（`auth.ErrLastAdmin`）；兩個 admin 同時互相停用只會成功一個。所有 admin 都登不進來時，`make recover-admin` 在主機上替既有的 admin 設新密碼、啟用並清掉節流。測試：`TestLastAdminCannotBeDisabled`、`TestConcurrentAdminDisableLeavesOne`、`TestRecoverAdmin` | 已緩解 |
| 其他 operation 不限次數 | 每 IP 限流只涵蓋 `CreateUser`、`CreateSession`；其他 operation 都要登入 | 範本接受；衍生專案依需要加進清單（`docs/rules/api-design.md` 限流） |

## Elevation of privilege（權限提升）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 一般使用者呼叫 admin 操作 | `operationAccess` 列出每個 operation 誰能呼叫；漏列的 operation、列了不存在的 operation、或「公開」與契約的 `security: []` 不一致，server 啟動就失敗（`adminOperations`）。`requireRole` 在 handler 之前以 403 拒絕。測試：`TestAdminOperationsChecksTable`、`TestOperationAccessEnforced`（逐一呼叫契約裡每個 operation：沒登入要 401，一般使用者呼叫 admin 操作要 403） | 已緩解 |
| 角色變更後仍用舊角色 | 每個請求都從資料庫讀角色。目前沒有改角色或改密碼的 operation；後端規則 8 要求這類功能撤銷該使用者所有 session（`backend/AGENTS.md`） | 已緩解 |
| 匿名請求到達受保護的 handler | 契約宣告 security，validator 在參數與 body 之前檢查。測試：`TestUnauthenticatedBeforeValidation`、`TestOperationAccessEnforced` | 已緩解 |
| backend 被入侵後改 schema | app 角色只有 DML。測試：`TestAppRoleCannotDDL` | 已緩解；所有表的資料仍可寫入，見「已接受的風險」 |
| 偷到或忘了登出的 session 被拿來刪除帳號 | 刪除自己的帳號要求 session 在 10 分鐘內登入過（`auth.ReauthenticateWithin`），前端先用密碼重新登入再刪除。測試：`TestDeleteCurrentUser`、`delete-account.test.tsx` | 已緩解 |
| 透過 API 建立 admin | 註冊一律寫 `user`；`EnsureAdmin` 只在 `seed-admin` 呼叫。測試：`TestSeedAdminCreatesAdmin` | 已緩解 |

## 已接受的風險

| 風險 | 為什麼接受 | 何時重新評估 |
|---|---|---|
| 每 IP 限流的狀態在程式記憶體裡，每個 backend 副本各有一份額度（`httpapi/ratelimit.go`） | 範本只跑一個 backend 副本 | 跑超過一個 backend 副本，或改用 Kamal、Swarm 部署：把限流移到資料庫或 proxy（ADR 基準的 Revocation Triggers） |
| 所有模組共用一個 app 角色與 `public` schema，app 角色能寫每張表（`migrate.go`） | 單一程序的模組化單體；分層由程式碼檢查（`backend/AGENTS.md` 規則 2、`internal/archtest`） | 某個模組拆成獨立服務，或需要自己的資料庫帳密 |
| 外洩密碼檢查查詢失敗時放行，且預設關閉 | 外部服務故障不該擋住註冊；內建清單一律檢查 | 產品要求外洩檢查是硬性規則 |
| 註冊會透露帳號名稱已被使用 | 用帳號名而不是 email，沒有 email 服務（ADR D6） | 加入 email 驗證或密碼找回 |
| 沒有改密碼、重設或找回密碼的 operation | 沒有 email 服務（ADR D6） | 同上；新 operation 必須撤銷 session（後端規則 8） |

## 何時更新這份文件

- `backend/api/openapi.yaml` 新增與認證有關的 operation，或 `operationAccess`、限流清單（`httpapi/server.go`）有增減。
- 新增角色，或改變授予角色的方式。
- Session 期限、cookie 名稱或屬性、token 格式改變。
- 新增外部身分提供者、MFA、Bearer token 或任何非瀏覽器的 client。
- `TRUSTED_PROXIES`、backend 前面的 proxy、backend 副本數改變。
- 上面某個 Open 項目處理完：改狀態並寫出測試。
