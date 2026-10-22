# 個資盤點

範本儲存、記錄或送出的個人資料：為什麼要、留多久、誰讀得到。描述的是出貨時的程式碼；衍生專案新增欄位、資料表或外部處理者時，在同一個改動裡更新這份文件。同一批資料的威脅與緩解：[威脅建模](../security/threat-model-auth.md)。

路徑相對於 repo 根目錄；`auth/` 指 `backend/internal/modules/auth/`，`httpapi/` 指 `backend/internal/httpapi/`。標「推論」的項目依據的是函式庫或工具的文件預設值，不是這個 repo 的程式碼。

## 誰讀得到資料庫

| 讀取者 | 權限 | 出處 |
|---|---|---|
| App 角色（`DB_APP_USER`，預設 `fullstack_app`） | `public` 每張表的 SELECT、INSERT、UPDATE、DELETE；`serve` 與 `seed-admin` 使用 | `deploy/postgres/initdb/10-roles.sh`、`backend/internal/platform/migrate/migrate.go` |
| Owner 角色（`DB_OWNER_USER`，預設 `fullstack_owner`） | 擁有資料庫與 `public`；只有 `app migrate` 使用 | `10-roles.sh` |
| Superuser（`POSTGRES_USER`） | 全部；初始化、維護與 `docs/deploy-vps.md` 第 7 節的備份腳本 | `10-roles.sh`、`docker-compose.yml` |
| 主機上能操作 Docker 的人 | 容器日誌與 volume | `docker-compose.yml` |

`PUBLIC` 對資料庫與 schema 沒有任何權限。

## 資料項目

| 資料 | 存在哪裡 | 用途 | 保留與刪除 | 誰讀得到 |
|---|---|---|---|---|
| 帳號名稱 | `users.account`，小寫，3 到 50 字元 | 登入識別 | 直到帳號被刪除（見「當事人的請求」） | 本人（`getCurrentUser`、`getUser`）；admin（`listUsers`、`getUser`、`patchUser` 的回應）；上述 DB 角色 |
| 密碼雜湊 | `users.password_hash`：對 NFKC 正規化後的密碼做 argon2id，m=19 MiB、t=2、p=1、16 byte salt、32 byte key（`auth/password.go`） | 驗證登入 | 直到該列被刪；API 從不回傳 | 只有 DB 角色 |
| 角色、停用狀態、建立與更新時間 | `users.role`、`is_disabled`、`created_at`、`updated_at` | 授權、帳號狀態 | 直到該列被刪 | `updated_at` 只有 DB 角色；其餘本人與 admin 可透過 API 讀 |
| Session token | 只在 cookie；資料庫存 token 的 SHA-256（`sessions.token_hash`） | 驗證請求 | cookie 的 `Max-Age` 是絕對期限。這些情況會刪掉該列：登出、全部登出、撤銷單一 session、帶著它重新登入、使用者被停用、使用者被刪（`ON DELETE CASCADE`）、每小時清理過期的 session（`auth/sessions.go`） | 原始 token 只有瀏覽器（HttpOnly）；雜湊只有 DB 角色 |
| Session 中繼資料 | `sessions.id`、`user_id`、`created_at`、`last_used_at`（最多每分鐘寫一次）、`expires_at` | 列出與撤銷 session；idle 與絕對期限 | 同 session 那列。超過 `expires_at` 或閒置超過 `SESSION_IDLE` 的列每小時清掉（`backend/cmd/app/main.go` 的 cleanup）。測試：`TestCleanup` | 本人，只限自己的 session；admin 沒有 session API；DB 角色 |
| 登入嘗試紀錄 | `sign_in_throttles`：`account_key` = 送出的帳號名轉小寫，不論帳號是否存在；`client_key` = 來源位址（IPv6 截到 /64）或信任裝置的 id，帳號合計那列為空字串；另有 `failures`、`next_allowed_at`、`updated_at` | 依「帳號＋來源」節流，與帳號層級的鎖 | 登入成功或 admin 解鎖時刪掉該帳號的所有列，或最後一次嘗試 24 小時後由每小時的清理刪除。測試：`TestCleanup` | DB 角色；admin 經 `GET /api/users/{id}/sign-in-lock` 只看得到是否鎖定與失敗次數。注意：打錯的帳號名、或誤把密碼打進帳號欄，會在這裡留最多 24 小時 |
| Items（範例模組） | `items.title`、`items.description`、`owner_id` | 範例的使用者內容；自由文字可能含個資 | `deleteItem` 刪除；帳號刪除時一起刪（`ON DELETE CASCADE`） | 只有擁有者（查詢帶擁有者條件，`backend/AGENTS.md` 規則 6）；DB 角色 |
| Client IP | backend access log 的 `client_ip`；每 IP 限流放在程式記憶體，IPv6 截到 /64（`httpapi/ratelimit.go`） | 追蹤、限流 | 日誌：見「日誌」。記憶體：閒置 1 分鐘的 bucket 會丟掉，重啟就全部消失 | 讀得到日誌的人 |
| Request id、method、path、status、處理時間 | backend access log；同一個請求的每行日誌都帶 `request_id` | 追蹤 | 見「日誌」 | 讀得到日誌的人 |
| User agent | backend 不記（`accessLog` 沒有這個欄位）。Caddy 的 `log` 會記請求 header（含 `User-Agent`）、含 query string 的完整 URI 與來源位址，並遮蔽 `Cookie` 與 `Authorization`（推論，Caddy 預設） | Proxy access log | 見「日誌」 | 讀得到日誌的人 |
| 送給 HIBP 的密碼雜湊前綴 | 只在 `PASSWORD_HIBP_CHECK=true`（預設 false）時：正規化後密碼的 SHA-1 前 5 個 hex 字元，帶 `Add-Padding: true`（`backend/internal/platform/hibp/hibp.go`）。測試：`TestBreached` | 註冊與 `seed-admin` 時的外洩密碼檢查 | backend 不儲存；處理者端的保留依它的政策 | Have I Been Pwned |
| 信任的裝置 | `trusted_devices`：裝置 cookie token 的 SHA-256、id、user id、建立與到期時間 | 本人曾登入過的瀏覽器不受帳號層級的鎖影響 | 180 天後到期並由每小時的清理刪除；全部登出、停用、救援、刪除帳號時刪除。測試：`TestCleanup`、`TestSignOutEverywhereForgetsDevices` | 原始 token 只有瀏覽器（HttpOnly）；雜湊只有 DB 角色 |
| 稽核紀錄 | `audit_events`：時間、執行者的 user id（主機上的指令為空）、動作、對象的 user id、request id；不存帳號名 | 追查誰停用、啟用、刪除、解鎖了哪個帳號 | 只能新增與讀取（app 角色沒有 UPDATE、DELETE）。超過 365 天的列在下一次 `app migrate`（每次部署）時刪除：長期沒有部署時會留得更久（表的 comment，`internal/platform/migrate`）；帳號刪除後紀錄仍在，只剩 id | 只有 DB 角色，沒有 API |
| 建立 admin 時的帳號名 | `seed-admin` 印到 stdout（`backend/cmd/app/seed.go`） | 回饋給操作者 | 看操作者的終端機或 job 輸出存到哪 | 操作者 |

## 瀏覽器端儲存

| 項目 | 由誰設定 | 內容 | 存活時間 |
|---|---|---|---|
| `__Host-session`（`COOKIE_SECURE=false` 時是 `session`） | backend（`httpapi/auth.go`） | Session token | Session 絕對期限，預設 7 天；登出或 server 判定無效時清除 |
| `__Host-device`（`COOKIE_SECURE=false` 時是 `device`） | backend，登入成功且瀏覽器還沒有這個帳號的裝置 cookie 時 | 信任裝置的 token | 180 天；全部登出、停用或刪除帳號後失效 |
| 語系 cookie | next-intl middleware（`frontend/src/proxy.ts`、`frontend/src/i18n/routing.ts`） | 選擇的語系（推論，next-intl 預設；routing 沒設 cookie 選項） | 函式庫預設 |

前端程式碼沒有使用 `localStorage`、`sessionStorage`、IndexedDB 或 `document.cookie`（搜尋 `frontend/src`）。Next 的 server 端讀請求 cookie，只為了轉送給 `GET /api/users/me`（`frontend/src/features/auth/server.ts`）。

## 日誌

**記了什麼**

- Backend：每個請求一行，含 `method`、`path`（不含 query string）、`status`、`duration_ms`、`client_ip`、`request_id`。`MODE=prod` 時是 JSON，其他是文字，寫到 stdout。路徑裡有資源 id，例如 session 或使用者的 UUID。
- Backend 錯誤：非預期錯誤記 `method`、`path` 與錯誤文字（`httpapi/problem.go`）。HIBP 查詢失敗會記錄錯誤；網路錯誤時那是 Go 的 `*url.Error`，含請求網址，也就是那 5 個字元的雜湊前綴（推論自 `net/http`）。
- 密碼與 session token 不會出現在任何級別的日誌：`TestCredentialsNeverLogged`。
- 清理：只記刪掉的筆數。
- 安全事件：註冊、登入成功與失敗（原因，帳號存在時帶 user id，不記送出的帳號名）、登出、被拒絕的 admin 操作，各一行帶 `event` 與 `user_id`（`TestSecurityEventsLogged`）。
- Caddy：每個請求一行 access log，內容見上表（推論）。
- PostgreSQL：image 預設值；違反唯一約束這類錯誤訊息可能帶出衝突的值，例如帳號名（推論，PostgreSQL 預設）。

**保留期間。** `docker-compose.yml` 讓每個容器的日誌輪替：每份最多 10 MB、最多 5 份，超過就丟掉最舊的（`x-logging`；CI 的 compose 檢查擋下沒設定的新服務）。實際保留多久取決於流量。`docs/deploy-vps.md` 第 7 節的備份腳本保留 7 天的完整資料庫 dump。

**操作者的責任。** 處理真實使用者的資料前，操作者要依流量確認輪替設定對應的保留期間、決定誰能在主機上操作 Docker，並在刪除使用者資料時一併處理備份。

## 當事人的請求

| 請求 | 現在有什麼 | 缺口 |
|---|---|---|
| 查閱 | 本人可讀自己的帳號（`getCurrentUser`）與 session（`listSessions`） | 沒有一次匯出 items 或其他模組資料的方式 |
| 更正 | 沒有：沒有改帳號名或密碼的 operation | 使用者需要更正資料時，衍生專案要補 |
| 刪除 | 本人在設定頁輸入密碼後刪除帳號（重新登入，再 `DELETE /api/users/me`；session 必須是 10 分鐘內登入的）；admin 用 `DELETE /api/users/{id}`。資料庫刪掉帳號、session 與每個模組擁有的列（指向 `users` 的 FK 都是 `CASCADE` 或 `SET NULL`，`TestForeignKeysToUsersAllowDeletion`）。`sign_in_throttles` 以帳號名為鍵，24 小時內過期。測試：`TestDeleteCurrentUser`、`TestAdminDeletesUser` | 備份保留 7 天，期間仍有被刪帳號的資料；日誌依輪替設定過期。衍生專案在 PostgreSQL 以外存放使用者資料（檔案、搜尋索引）時，要在刪除帳號時一併處理 |
| 限制處理 | admin 停用（`patchUser`） | 範本無 |

## 何時更新這份文件

- 新增存放個資的資料表或欄位，或改變既有資料的保留與刪除方式。
- 新增日誌欄位、新增帶有請求或使用者資料的日誌，或改變日誌保留期間。
- 新增外部處理者（email、分析、錯誤追蹤、身分提供者），或改變送給 HIBP 的內容。
- 前端新增 cookie 或瀏覽器儲存。
- 新增匯出、更正或刪除使用者資料的 operation。
