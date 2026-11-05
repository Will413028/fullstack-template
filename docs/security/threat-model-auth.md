# 威脅建模：認證與 session

用 STRIDE 逐項檢查帳號、密碼、session 與角色在目前程式碼裡的做法。每個緩解措施都寫出程式碼或證明它的測試；找不到程式碼的威脅列為 Open。設計的決定在 [API 與認證基準](../adr/2026-10-18-api-and-auth-baseline.md) D4、D5、D6、D7（session、CSRF、密碼與節流、授權）與 [Go 後端](../adr/2026-10-18-go-backend.md) D5（資料庫角色）。

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

1. 瀏覽器 → Vercel：頁面與 `/api/*` 同源；Vercel 覆寫 `X-Real-IP` 與 `X-Forwarded-For`，client 帶的值進不來（[ADR 前端改由 Vercel 部署](../adr/2026-11-03-frontend-on-vercel.md)）。
2. Vercel → Caddy（`frontend/vercel.json`）：經公開網路，Caddy 在 API 的網域上有 TLS 與 HSTS。只有帶對 `X-Origin-Secret`（`.secrets/origin_secret`，只放在 Vercel 的 Production 環境）的請求會被轉給 backend，其餘回 403（`deploy/Caddyfile`）。只有 Caddy 發佈 port（`docker-compose.yml`）。
3. Caddy → backend，走 compose 網路。Caddy 只在密鑰相符時，以 `X-Real-IP` 覆寫 `X-Forwarded-For`、以 `X-Forwarded-Host` 覆寫 `Host`；backend 只相信 `TRUSTED_PROXIES`（預設是 Caddy 的固定位址）送來的 `X-Forwarded-For` 與 `X-Request-ID`。
4. Next server（Vercel Function）→ API：server 端呼叫打網站自己的 `/api`（`NEXT_PUBLIC_APP_URL`），經 Vercel edge，只轉送請求的 cookie（`frontend/src/features/auth/server.ts`）。Next 的程式碼不持有密鑰，`X-Real-IP` 由 edge 填寫，所以它無法指定 client 位址。
5. backend → PostgreSQL：用 app 角色，只有 DML 權限（`deploy/postgres/initdb/10-roles.sh`、`backend/internal/platform/migrate/migrate.go`）。
6. backend → Have I Been Pwned range API：只在 `PASSWORD_HIBP_CHECK=true` 時（`backend/internal/platform/hibp/hibp.go`）。

**入口**（`api/` 的 Connect service，路徑是 `/api/app.<package>.<Service>/<Method>`；掛載清單是 `httpapi/server.go` 的 `connectServices`；[ADR 對外 API 改用 Protobuf＋Connect](../adr/2026-11-03-api-contract-protobuf-connect.md)）

| Service | Method | 誰能呼叫 |
|---|---|---|
| `app.users.v1.UserPublicService` | `CreateUser` | 匿名；每個 IP 限流 |
| `app.sessions.v1.SessionPublicService` | `CreateSession`、`DeleteCurrentSession` | 匿名；`CreateSession` 每個 IP 限流、每個帳號節流；`DeleteCurrentSession` 讓過期的 cookie 也能登出 |
| `app.users.v1.UserAppService` | `GetCurrentUser`、`GetUser`、`DeleteCurrentUser` | 登入者，只限自己（`GetUser` 別人的 id 是 `not_found`）；`DeleteCurrentUser` 的 session 必須是 10 分鐘內登入的，否則 `permission_denied`（`REAUTHENTICATION_REQUIRED`） |
| `app.sessions.v1.SessionAppService` | `ListSessions`、`GetSession`、`GetCurrentSession`、`DeleteSession`、`DeleteAllSessions` | 登入者，只限自己的 session |
| `app.items.v1.ItemAppService` | `CreateItem`、`GetItem`、`ListItems`、`UpdateItem`、`DeleteItem` | 登入者，只限自己的 item |
| `app.admin.users.v1.UserAdminService` | `ListUsers`、`GetUser`、`UpdateUser`、`DeleteUser`、`GetSignInLock`、`ResetSignInLock` | 只有 admin |

誰能呼叫跟著 service 的名稱走：`<Module>PublicService` 匿名、`<Module>AppService` 登入者、`<Module>AdminService` 只有 admin；名稱不屬於這三類的 service 不能掛載，server 啟動就失敗（`rpc.NewMux`）。App 與 Admin 的匿名呼叫、入口限流在 net/http 這一層、Connect 解碼請求之前處理（`httpapi/server.go` 的 `connectEntry`）；admin 的角色由 interceptor 檢查（`rpc.NewMux`）。外層的保護（request id、存取日誌、安全標頭、panic、CrossOriginProtection、body 上限）包住所有請求。不是 procedure 的路徑（`/api` 底下或其他）一律回 Connect 的 `not_found`。另有測試用的 service（`httpapi/rpc/testdata`），只在測試裡掛載。

不在範圍內：`items` 的各個 operation（擁有者範圍之外）、主機與 Docker 的強化、Caddy 的 TLS 設定。

## Spoofing（冒充）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 對單一帳號線上猜密碼 | 每個「帳號＋來源」（位址，IPv6 以 /64；或信任的裝置）各自節流，在比對密碼之前計數，第 5 次失敗起等待時間加倍、上限 1 小時；沒有該帳號裝置 cookie 的來源另外合計，連續失敗 100 次（NIST SP 800-63B 3.2.2）起全部拒絕，直到登入成功、admin 解鎖或 24 小時沒有嘗試（`auth/devices.go`、`auth/queries/sign_in_throttles.sql`）。測試：`TestSignInThrottlePerClient`、`TestUntrustedClientsLockedTrustedDeviceNot`、`TestLoginThrottlePerAccount`、`TestLoginThrottleConcurrent`、`TestLoginThrottleSchedule` | 已緩解 |
| 從同一位址對多個帳號噴灑密碼 | `CreateUser`、`CreateSession` 的每 IP token bucket，預設每分鐘 5 次，IPv6 以 /64 為鍵（`httpapi/ratelimit.go`；清單是 `httpapi/server.go` 的 `connectLimited`）。在入口、Connect 解碼之前扣：格式錯誤的請求也用掉額度。測試：`TestRateLimitAuthOperations`、`TestIPLimiterKeysIPv6By64` | 單一副本內已緩解；見「已接受的風險」 |
| 弱密碼或外洩過的密碼 | 15 到 128 字元（下限以 NFKC 正規化後的長度計、上限以原始輸入計；`auth/limits.go`，HTTP、`seed-admin` 與 `recover-admin` 共用）、NFKC 正規化、內建常見密碼清單、可選的 HIBP 檢查（`auth/auth.go`、`auth/password.go`）。測試：`TestCreateUserLimits`、`TestSeedAdminRejectsShortPassword`、`TestRecoverAdminRejectsShortPassword`、`TestCreateUserCommonPassword`、`TestCreateUserBreachedPassword`、`TestPasswordNormalized`、`TestCheckNewAccountPasswordLengthAfterNFKC` | 已緩解。HIBP 預設關閉，查詢失敗時放行：接受，外部服務故障不該擋住註冊 |
| 猜 session token | `crypto/rand` 產生的 256-bit token（`auth/sessions.go`）。測試：`TestCreateUserSetsSessionCookie` | 已緩解 |
| Session fixation | 每次登入都發新 token，並刪掉請求帶來的舊 token。測試：`TestCreateSessionIssuesNewToken` | 已緩解 |
| 登出、停用或過期後重用 token | 每個請求都重讀 session 與使用者；idle 預設 1 天、絕對期限 7 天；停用會刪掉該使用者所有 session。測試：`TestSessionIdleExpiry`、`TestSessionAbsoluteExpiry`、`TestDeleteCurrentSessionRevokes`、`TestDeleteAllSessions`、`TestAdminDisablesUserRevokesSessions`、`TestMeDisabledAccount`、`TestMeAfterUserDeleted` | 已緩解 |
| 用 header 代替 cookie 送 token | 只讀 cookie（`httpapi/auth.go`）。測試：`TestMeIgnoresBearer` | 已緩解 |
| Client 偽造位址規避每 IP 限流 | `X-Forwarded-For` 從右往左走，只經過受信任的 proxy（`httpapi/middleware.go`）；Caddy 只在 origin secret 相符時採信 Vercel 填的 `X-Real-IP`，並蓋掉 client 帶的 `X-Forwarded-For`（`deploy/Caddyfile`）。測試：`TestClientIP`、`TestRateLimitIgnoresXFFFromUntrustedPeer`、`TestRateLimitPerIPBehindTrustedProxy`；CI compose job「Caddy hands the backend Vercel's client address…」 | `TRUSTED_PROXIES` 只列 proxy、密鑰沒外洩時已緩解 |
| 繞過 Vercel 直接打 API 網域 | 沒帶對 `X-Origin-Secret` 回 403；密鑰檔是空的或不存在時一律拒絕（長度檢查、容器起不來）。CI compose job「Caddy passes only requests that carry the origin secret」 | 已緩解；輪替程序見 `docs/deploy-vps.md` |
| 被停用的帳號登入 | 回應與密碼錯誤相同的 `invalid_credentials`。測試：`TestCreateSessionDisabledAccount` | 已緩解 |

## Tampering（竄改）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 跨站請求偽造（CSRF） | `http.CrossOriginProtection` 包住整個 mux；cookie 是 `SameSite=Lax`（`httpapi/server.go`、`httpapi/auth.go`）。Caddy 把 `Host` 換成網站的 host，沒有 `Sec-Fetch-Site` 的舊瀏覽器比對 `Origin` 與 `Host` 時才不會誤擋。測試：`TestCrossOriginPostRejected`、`TestCreateItemCrossSite`（Connect，`permission_denied`／`CROSS_ORIGIN_REQUEST`）；CI compose job 的同站 `CreateSession` | 已緩解。`Sec-Fetch-Site` 與 `Origin` 都沒帶的請求會放行；瀏覽器在會改變狀態的請求上都會帶 |
| 註冊時自己指定角色 | `CreateUserRequest` 只有帳號與密碼，請求裡多帶的欄位（例如 `role`）被丟掉（connect-go 的 `DiscardUnknown`）；service 一律寫入 `user`。測試：`TestCreateUserLimits`（帶 `role` 註冊得到 `ROLE_USER`）、`TestCreateUserSetsSessionCookie`（角色是 `user`） | 已緩解 |
| 同一帳號名稱同時註冊 | 由唯一索引決定，恰好一個成功，其餘 `already_exists`。測試：`TestCreateUserConcurrentSameAccount` | 已緩解 |
| Token 走明文 http | 除非 `COOKIE_SECURE=false`，cookie 帶 `Secure` 與 `__Host-` 前綴；網站網域的 HSTS 由 Vercel 送，API 網域的由 Caddy 送。測試：`TestCookieNameFollowsSecure`、`TestConfigDefaults` | `SITE_ADDRESS` 是網域時已緩解；`:80` 只供本機使用 |
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
| 登入時從回應或時間差列舉帳號 | 不存在的帳號也跑一次 dummy hash，回同樣的錯誤（`unauthenticated`、`INVALID_CREDENTIALS`）；節流也套用在不存在的帳號名。測試：`TestCreateSessionUnknownAccountRunsHash`、`TestLoginThrottlePerAccount` | 已緩解 |
| 註冊時列舉帳號 | 帳號已存在回 `already_exists`（`ACCOUNT_TAKEN`）。測試：`TestCreateUserDuplicate` | 接受：用帳號名而不是 email，無法隱藏名稱已被使用（ADR api-and-auth-baseline D6）；註冊有每 IP 限流 |
| 讀到別人的使用者資料或 session | 非擁有者一律 404（`auth/auth.go`、`auth/queries/sessions.sql`）。測試：`TestGetOtherUserNotFound`、`TestDeleteOtherUsersSession`、`TestNonAdminCannotListUsers` | 已緩解 |
| 內部錯誤洩漏細節 | 非預期錯誤變成不帶細節的 `internal`，細節只進日誌（`httpapi/rpc/errors.go` 的 `Convert`、`LogConverted`）。測試：`TestConvertUnexpectedIsInternal`、`TestInternalErrorLogged`、`TestUnhandledErrorHidden` | 已緩解 |
| 密碼送到 HIBP | 只送 SHA-1 的前 5 個 hex 字元，帶 `Add-Padding`（`hibp.go`）。測試：`TestBreached` | 已緩解。HIBP 網路錯誤的訊息含請求網址，也就是同樣的 5 個字元，會寫進本機日誌（推論自 `net/http` 的 `*url.Error`）：接受，內容與送出去的相同 |
| backend ↔ PostgreSQL 傳輸未加密 | 連線必須 `sslmode=verify-full`；只有設定 `DATABASE_PLAINTEXT=same-host` 才接受不加密，範本只在 compose 裡設定（同一台主機的內部網路，資料不出主機）。改接遠端資料庫卻沒加密時，backend 與 migrate 拒絕啟動（`internal/platform/config`）。測試：`TestDatabaseURLRequiresVerifiedTLS` | 已緩解。compose 內部不加密是接受的取捨：要防同一網路上被入侵的容器，或資料庫搬到另一台主機時，改用 TLS |

## Denial of service（阻斷服務）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 大量雜湊耗盡記憶體或 CPU | 同時最多 `runtime.NumCPU()` 個 argon2id 雜湊，等待者隨 context 結束（`auth/password.go`）。測試：`TestArgonConcurrencyBounded` | 已緩解 |
| 過大或過慢的請求 | body 上限 1 MiB；header、讀、寫逾時（`httpapi/server.go`）。測試：`TestBodyTooLarge`、`TestSlowHeaderClosed` | 已緩解 |
| 故意登入失敗把受害者鎖在門外 | 節流以「帳號＋來源」為單位，攻擊者的失敗只讓自己的來源等待；帳號層級的鎖只擋沒有該帳號裝置 cookie 的來源，本人曾登入過的瀏覽器照常登入（OWASP「Slow Down Online Guessing Attacks with Device Cookies」）；admin 可查看與解鎖（`UserAdminService.GetSignInLock`、`ResetSignInLock`，寫入稽核）。全部登出、停用、救援時忘掉該帳號的裝置。測試：`TestUntrustedClientsLockedTrustedDeviceNot`、`TestDeviceCookieIsPerAccount`、`TestSignOutEverywhereForgetsDevices`、`TestAdminUnlocksSignIn` | 已緩解。本人在新裝置上仍可能被鎖，要等 admin 解鎖；加了 email 之後改成鎖定期間可用重設密碼解鎖（ADR api-and-auth-baseline D6 的重新評估條件） |
| 用亂數帳號名塞滿 `sign_in_throttles` | 不存在的帳號名也會建立紀錄（每次嘗試最多兩列：來源一列、帳號合計一列）；每 IP 限流限制速率，閒置 24 小時的紀錄每小時清掉。測試：`TestCleanup` | 單一副本內已緩解 |
| Admin 停用最後一個 admin，包括自己 | 停用時先鎖住所有啟用中的 admin，目標是唯一一個就回 `failed_precondition`（`LAST_ADMIN`）（`auth.ErrLastAdmin`）；兩個 admin 同時互相停用只會成功一個。所有 admin 都登不進來時，`make recover-admin` 在主機上替既有的 admin 設新密碼、啟用並清掉節流。測試：`TestLastAdminCannotBeDisabled`、`TestConcurrentAdminDisableLeavesOne`、`TestRecoverAdmin` | 已緩解 |
| 其他 operation 不限次數 | 每 IP 限流只涵蓋 `CreateUser`、`CreateSession`；其他 operation 都要登入 | 範本接受；衍生專案依需要加進清單（`docs/rules/api-design.md` 限流） |

## Elevation of privilege（權限提升）

| 威脅 | 程式碼裡的緩解 | 狀態 |
|---|---|---|
| 一般使用者呼叫 admin 操作 | service 名稱決定類別，名稱不屬於三類就不能掛載（`rpc.NewMux`）；`AdminService` 的方法由 interceptor 檢查 admin 角色，以 `permission_denied`（`INSUFFICIENT_ROLE`）拒絕並記 `authz.denied`。測試：`TestProcedureAccessEnforced`（逐一呼叫 `connectServices` 掛上的每個 procedure：App 與 Admin 沒登入要 `unauthenticated`，一般使用者呼叫 Admin 要 `permission_denied`）、`TestConnectAdminServiceNeedsAdminRole`、`TestNonAdminCannotListUsers` | 已緩解 |
| 一般使用者呼叫 Connect 的 Admin service | service 的名稱決定類別，不屬於三類的 service 啟動就失敗；Admin service 的每個方法由 interceptor 檢查 admin 角色，拒絕時回 `permission_denied`（`INSUFFICIENT_ROLE`）並記 `authz.denied`（`httpapi/rpc/interceptor.go` 的 `requireAdmin`）。測試：`TestAdminServiceNeedsAdminRole`、`TestConnectAdminServiceNeedsAdminRole`、`TestNewHandlerRejectsUnclassifiedService` | 已緩解 |
| 匿名請求在 Connect 解碼之前得到解碼或欄位錯誤 | connect-go 先解碼才跑 interceptor，所以未登入檢查放在 net/http 層：App 與 Admin 的方法在 Connect handler 之前回 `unauthenticated`；不存在的 procedure 一律 `not_found`，不先要求登入。測試：`TestConnectUnauthenticatedBeforeDecoding`、`TestConnectUnknownProcedureNotFound` | 已緩解 |
| 用 GET 繞過 CSRF 檢查呼叫有副作用的方法 | CrossOriginProtection 不檢查 GET；只有 `Get*`、`List*` 方法可以標 `NO_SIDE_EFFECTS`（connect-go 只對這種方法接受 GET），其他的標了就啟動失敗（`rpc.NewMux`）。測試：`TestNewHandlerRejectsSideEffectsOnGet`、`TestCrossOriginRejectedAsPermissionDenied` | 已緩解 |
| 角色變更後仍用舊角色 | 每個請求都從資料庫讀角色。目前沒有改角色或改密碼的 operation；後端規則 8 要求這類功能撤銷該使用者所有 session（`backend/AGENTS.md`） | 已緩解 |
| 匿名請求到達受保護的 handler | App 與 Admin 的匿名呼叫在 net/http 層、Connect 解碼之前回 `unauthenticated`（`connectEntry`），看不到解碼錯誤或欄位錯誤。測試：`TestUnauthenticatedBeforeValidation`、`TestConnectUnauthenticatedBeforeDecoding`、`TestProcedureAccessEnforced` | 已緩解 |
| backend 被入侵後改 schema | app 角色只有 DML。測試：`TestAppRoleCannotDDL` | 已緩解；所有表的資料仍可寫入，見「已接受的風險」 |
| 偷到或忘了登出的 session 被拿來刪除帳號 | 刪除自己的帳號要求 session 在 10 分鐘內登入過（`auth.ReauthenticateWithin`），前端先用密碼重新登入再刪除。測試：`TestDeleteCurrentUser`、`delete-account.test.tsx` | 已緩解 |
| 透過 API 建立 admin | 註冊一律寫 `user`；`EnsureAdmin` 只在 `seed-admin` 呼叫。測試：`TestSeedAdminCreatesAdmin` | 已緩解 |

## 已接受的風險

| 風險 | 為什麼接受 | 何時重新評估 |
|---|---|---|
| 每 IP 限流的狀態在程式記憶體裡，每個 backend 副本各有一份額度（`httpapi/ratelimit.go`） | 範本只跑一個 backend 副本 | 跑超過一個 backend 副本，或改用 Kamal、Swarm 部署：把限流移到資料庫或 proxy（ADR 基準的 Revocation Triggers） |
| 所有模組共用一個 app 角色與 `public` schema，app 角色能寫每張表（`migrate.go`） | 單一程序的模組化單體；分層由程式碼檢查（`backend/AGENTS.md` 規則 2、`internal/archtest`） | 某個模組拆成獨立服務，或需要自己的資料庫帳密 |
| origin secret 是共用的靜態密鑰，只在外洩或人員異動時手動輪替（Caddy 輪替期間同時接受新舊兩個值） | Vercel 沒有能驗證來源的機制，也沒有固定的出口 IP | Vercel 提供 mTLS 或簽章（ADR 前端改由 Vercel 部署） |
| 外洩密碼檢查查詢失敗時放行，且預設關閉 | 外部服務故障不該擋住註冊；內建清單一律檢查 | 產品要求外洩檢查是硬性規則 |
| 註冊會透露帳號名稱已被使用 | 用帳號名而不是 email，沒有 email 服務（ADR api-and-auth-baseline D6） | 加入 email 驗證或密碼找回 |
| 沒有改密碼、重設或找回密碼的 operation | 沒有 email 服務（ADR api-and-auth-baseline D6） | 同上；新 operation 必須撤銷 session（後端規則 8） |

## 何時更新這份文件

- `api/` 新增與認證有關的 service 或方法、限流清單（`httpapi/server.go` 的 `connectLimited`）有增減；`connectServices` 掛上新的 service。
- 新增角色，或改變授予角色的方式。
- Session 期限、cookie 名稱或屬性、token 格式改變。
- 新增外部身分提供者、MFA、Bearer token 或任何非瀏覽器的 client。
- `TRUSTED_PROXIES`、backend 前面的 proxy（Caddy、Vercel 的轉送設定）、backend 副本數改變。
- 上面某個 Open 項目處理完：改狀態並寫出測試。
