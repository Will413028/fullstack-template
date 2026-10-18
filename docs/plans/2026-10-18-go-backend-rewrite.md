# 後端由 FastAPI 改寫成 Go，並把 API 與認證改成業界基準

**Goal：** 完成後的狀態如下，驗收見步驟 8。
- `backend/` 是 Go 1.27 的 modular monolith：stdlib `net/http`、oapi-codegen strict server、spec 驗證 middleware、pgx＋sqlc、Atlas 宣告式 schema、DB opaque session、`log/slog`。
- API 掛在和前端同源的 `/api` 底下，形狀照業界基準：
  - 資源導向的 URL，回應不包外殼；
  - 建立回 201，刪除回 204；
  - keyset 分頁、JSON Merge Patch；
  - 錯誤用 RFC 9457，並帶機器可讀的 code；
  - 主鍵是 UUIDv7。
- 認證符合 OWASP 與 NIST 800-63B-4：`__Host-` cookie、`CrossOriginProtection` 擋 CSRF、依帳號節流、密碼比對常見與外洩清單、狀態變更時撤銷 session。
- 前端改用從 `openapi.yaml` 生成的型別與 client。
- CI、compose、部署文件、`make rename`、指令檔、規則文件全部更新。
- repo 內沒有 Python 後端殘留，也沒有舊契約的殘留。

**上層：** 主清單在 repo 外，見本機個人脈絡檔。沒有主計畫，由下一段暫代。

**附件：** [`2026-10-18-go-backend-rewrite/`](2026-10-18-go-backend-rewrite/)，快照 `7a4d0fa`：
- `inventory.md`：舊系統盤點與每一項的基準去向
- `test-map.md`：30 條舊測試與新增測試的對照
- `check_inventory.py`：盤點完整性與殘留檢查

**決策紀錄：**
- [`docs/adr/2026-10-18-go-backend.md`](../adr/2026-10-18-go-backend.md)：範圍、stack、資料層
- [`docs/adr/2026-10-18-api-and-auth-baseline.md`](../adr/2026-10-18-api-and-auth-baseline.md)：拓樸、API 形狀、認證

## 全域約束與完成定義（暫代主計畫）

- **分支與 PR：** 在 `go-rewrite` 分支上做。步驟 1 開 draft PR 到 `main`，之後每一步的「CI 綠燈」都以該 PR 的 `gh pr checks` 為準（CI 只在 PR 與 push `main` 時觸發，見 `.github/workflows/ci.yml:3-7`）。不留新舊並行的路徑，也不留相容 shim。
- **舊系統：** 保存在 tag `archive/pre-go-rewrite`。要對照時用 `git worktree add <scratchpad>/ft-py archive/pre-go-rewrite`，執行方式見步驟 7。
- **基準優先：** 每個設計點以 `inventory.md` 寫的基準為準。舊系統的形狀不構成理由；要偏離基準，先回報 Will。
- **依賴：** 只用步驟 0 核准的清單（P14）。清單外的套件要另外列出、查 deps.dev 發布日期，等 Will 核准（`AGENTS.md` Constraints）。30 天冷卻期的例外（D13）：Go toolchain、官方 base image 與已採用工具的安全修補版，修的是已公告的漏洞時可以用冷卻期內的版本，列出修掉的公告編號。
- **契約是唯一來源：** 長度、格式、enum 只寫在 `openapi.yaml`。後端由 validator 執行，前端由生成的型別取用，不在 handler 或前端重寫一次。DB CHECK 是第二道防線，數值和契約一致。
- **錯誤：** 一律在 `internal/httpapi` 一處轉成 RFC 9457 problem（I4）。
- **改寫期間的指令檔：** 步驟 1–5 期間，root `AGENTS.md` 的 Design Rules 與 Constraints 中的 Alembic 規則、`backend/AGENTS.md`、`docs/rules/*` 描述的是舊系統，**由本計畫取代**。步驟 1 在這些檔案開頭各加一行「Go 改寫中，以 `docs/plans/2026-10-18-go-backend-rewrite.md` 為準」，步驟 6 完整改寫並移除該行。
- **SQL：** 每個查詢都經 sqlc 生成。不手寫 migration；改 `schema.sql` 後用 `atlas migrate diff` 產生。
- **完成定義（每個模組）：**
  - 盤點表中屬於該模組的每一列，去向都已實作；
  - `test-map.md` 中該模組的每一列都有 Go integration 測試（testcontainers，PostgreSQL 18）；
  - 盤點 E 段屬於該模組的缺陷都有回歸測試；
  - 當下步驟定義的 `make -C backend check` 綠燈，draft PR 的 CI 綠燈。

## 前提（開工時核對過，2026-10-18）

- **P1　既有部署是否在線：不在線**（已確認）。範本本身沒有部署：`.github/workflows/` 只有 `ci.yml` 與 `security.yml`，沒有 deploy。已 clone 出去的專案與範本斷開（專案頁 Overview）。所以不受回滾、相容性窗口、資料搬移的約束；回滾就是不合併或 revert PR。
- **P2　動機與範圍**：對齊 FathomPod／dailyfresh 的 Go stack，直接取代 Python 後端，並把 API 與認證照業界基準重做，不保留舊契約（Will，2026-10-18，已確認）。
- **P3　消費端**：前端是唯一消費端，而且是我們擁有的程式，會隨這次改寫一起改（已確認）。
- **P4　Go 版本與 module path**（預設，可推翻）：
  - Go 1.27（本機 `go1.27.2`）。主鍵由 DB 的 `uuidv7()` 產生。Go 端一律用 `github.com/google/uuid.UUID`：oapi-codegen runtime 的 `openapi_types.UUID` 就是它，sqlc 的 uuid override 也設成它，全專案只有一種 UUID 型別。
  - module path 為 `github.com/Will413028/fullstack-template/backend`。
  - `make rename NAME=<n>` 預設把 module path 改成 `github.com/Will413028/<n>/backend`；另提供可選參數 `MODULE=<path>` 覆寫。
- **P5　HTTP 與契約**：
  - stdlib `ServeMux` 加上 oapi-codegen strict server，生成碼放在 `internal/httpapi/gen`；request 驗證用 oapi-codegen 的 nethttp middleware（kin-openapi）。
  - spec 的 `servers` 為 `- url: /api`，paths 不含 `/api` 前綴。validator 只掛在 `/api/` 子樹，並以 `Options.Skipper` 跳過 dev 文件路由（`/api/openapi.yaml`、`/api/docs`）；`/health`、`/health/ready` 掛在 validator 外層。找不到路由時 validator 回 404，這正是 `/api` 底下未定義路徑應有的行為。
  - 契約寫在 `backend/api/openapi.yaml`，用 OpenAPI 3.0.3。這是 REAL 約束：oapi-codegen v2.8.0 對 3.1 的支援仍標示為 initial。重新評估的觸發點：oapi-codegen 宣布 3.1 支援穩定。
  - 前端用 openapi-typescript 生成型別，搭配 openapi-fetch。
  - breaking-change 檢查用 oasdiff 對 `main` 比對，從本 PR 合併後才生效。
- **P6　Session**（OWASP Session Management；NIST AAL1）：
  - token：32 bytes `crypto/rand`，base64url；DB 只存 SHA-256。
  - 期限：absolute 7 日、idle 1 日；`last_used_at` 每分鐘最多寫一次。
  - cookie：`__Host-session`，屬性 `HttpOnly; Secure; Path=/; SameSite=Lax`，不帶 Domain。本機 http 開發（`COOKIE_SECURE=false`）時改名為 `session`。
  - 登入一律發新 token；停用帳號時撤銷該使用者的所有 session。範本沒有改 role、改密碼的功能；「改 role、改密碼時撤銷所有 session」寫成 `docs/rules/api-design.md` 的規則，供之後新增時遵守。
- **P7　密碼**（NIST 800-63B-4；OWASP Password Storage）：
  - 長度 15–128 個 code point，不設組合規則，比對內嵌清單；HIBP 為可選，預設關閉。
  - 雜湊用 argon2id，參數 m=19 MiB、t=2、p=1，存成 PHC 字串；同時進行的雜湊數上限為 `NumCPU`。
  - Unicode 正規化：NIST 800-63B-4 寫「SHOULD apply the normalization process for stabilized strings」（審查者 2026-10-18 以 WebFetch 查證）。照做：service 在雜湊之前做 NFKC（`golang.org/x/text/unicode/norm`）；長度限制由 validator 依原始輸入的 code point 計算。
- **P8　登入識別**：用 username，不分大小寫（存入前轉小寫）。不做 email、找回密碼、MFA，見「範圍」的延後項。
- **P9　開發熱重載**：用 `docker compose watch`（`develop.watch` 的 `rebuild`），不加 air（預設，可推翻）。
- **P10　部署拓樸**：同源。
  - 正式環境：Caddy 跑在 compose 內（`deploy/Caddyfile`、`caddy` service），`/api/*` 轉給 backend，其餘轉給 Next。只有 Caddy 發佈 80/443，HSTS 也在 Caddy 設定。`SITE_ADDRESS` 決定站點位址，本機 `make up` 用 `:80`，搭配 `COOKIE_SECURE=false`。
  - compose network 固定 subnet `172.30.0.0/24`，`TRUSTED_PROXIES` 設為這個 subnet。backend 不發佈 port，所以能連進 backend 的只有同 subnet 的 Caddy 與 Next。
  - dev：`next dev` 的 `rewrites`（只在 `NODE_ENV=development` 生效）轉發 `/api`；dev compose 的 port 綁 `127.0.0.1`。
  - 改用 compose 外的 proxy 時，要自行設定 `TRUSTED_PROXIES`，寫進 `docs/deploy-vps.md`。
- **P11　repo 工具維持 Python**：`scripts/*.py`（rename、init_env）是 repo 工具，不是後端（預設，可推翻）。
- **P12　本機 `.env` 由 Will 更新**（需要人做，見步驟 1）。agent 不讀 `.env`。新變數依 `.env-example`：
  - `DATABASE_URL` 去掉 `+asyncpg`；
  - 刪除 `SECRET_KEY`、`COOKIE_SAMESITE`、`COOKIE_DOMAIN`、`CORS_ORIGINS`、`NEXT_PUBLIC_API_URL`。
- **P13　步驟 0 查證的事實**（2026-10-18）：
  - **`http.Server` timeout 與 gosec G112：已確認。** 欄位有 `ReadTimeout`、`ReadHeaderTimeout`、`WriteTimeout`、`IdleTimeout`、`MaxHeaderBytes`。`ReadHeaderTimeout` 為 0 時沿用 `ReadTimeout`，兩者都是 0 就沒有 timeout；`IdleTimeout` 同理（[go1.27.2 `src/net/http/server.go:3089-3118`](https://raw.githubusercontent.com/golang/go/go1.27.2/src/net/http/server.go)）。G112 只檢查 `http.Server{...}` literal 裡有沒有寫 `ReadHeaderTimeout` 或 `ReadTimeout` 這個 key，不看值，事後賦值（`srv.ReadHeaderTimeout = …`）也不算（[gosec `rules/slowloris.go`](https://github.com/securego/gosec/blob/master/rules/slowloris.go)）。所以 G112 擋不住設成 0；B19 的 timeout 要靠 `TestSlowHeaderClosed` 守住，不能靠 lint。
  - **Docker 發佈的 port 繞過 ufw：已確認。** Docker 文件：發佈 port 後，封包「are routed before the firewall rules can be applied, effectively ignoring your firewall configuration」（[Packet filtering and firewalls](https://docs.docker.com/engine/network/packet-filtering-firewalls/)）。P10 的設計因此不變：只有 Caddy 發佈 port，dev compose 綁 `127.0.0.1`。
  - **Atlas community 版：不成立，已由 D12 處理。** 下載 URL `https://release.ariga.io/atlas/atlas-community-<os>-<arch>-<ver>` 確認是 community 版。但 FathomPod 在 v1.1.3 實測過：community 版不接受多個 schema 檔的 `src`，`concurrent_index` 產的檔也不加 `-- atlas:txmode none`（wiki `tech/atlas-migrate-ci-gate` Gotcha 6、Case 25；FathomPod ADR `2026-10-07-go-rewrite-scope-and-cutover`）。所以 FathomPod 改成產生 migration 與漂移檢查用 standard 版、image 內套用用 community 版（`backend/Dockerfile:15-27`、`.github/workflows/ci.yml:93-97`）。這和 I5 的「各模組一份 `schema.sql`」直接衝突，觸發步驟 0 的停止條件；Will 選擇照 FathomPod 分兩個版本別（D12）。
  - **Atlas 與 pgx 的 `sslmode` 預設：不同。** Atlas v1.3.0 用 `lib/pq` v1.12.0（`cmd/atlas/go.mod:20`），它的預設是 `require`（`connector.go:61`）；pgx v5.11.0 照 libpq 預設 `prefer`（`pgconn/config.go:296-302`）。compose 內的 PostgreSQL 沒開 TLS，所以 Atlas 不帶參數會連不上。改法：`.env-example` 的 `DATABASE_URL` 直接寫 `?sslmode=disable`，backend 與 migrate 共用同一個 URL，不另外傳一份（FathomPod `backend/deploy/compose.yaml:102` 也是這樣）。
  - **`http://localhost` 上的 `Secure` cookie：已確認，不影響設計。** Chrome 與 Firefox 接受；Safari 拒收 `Secure`、`__Host-`、`__Secure-`（[WebKit bug 231035](https://bugs.webkit.org/show_bug.cgi?id=231035)、[httpwg issue 2605](https://github.com/httpwg/http-extensions/issues/2605)，屬推論：bug 報告是 2021–2024 年的，沒有在當前 Safari 實測）。`COOKIE_SECURE=false` 時 cookie 改名為 `session`、不帶 `Secure`，三家都能用。

- **P14　核准的依賴**（Will 2026-10-18 核准；查證日 2026-10-18，冷卻期截止 2026-09-18；OSV 查過，只有 x/crypto 的 GO-2026-5932，影響 `openpgp`，本專案不用）。清單外的套件或換版本仍照全域約束「依賴」：
  - Go module：`github.com/jackc/pgx/v5` v5.11.0；`github.com/google/uuid` v1.6.0；`github.com/oapi-codegen/runtime` v1.7.0；`github.com/oapi-codegen/nullable` v1.2.0；`github.com/oapi-codegen/nethttp-middleware` v1.2.0；`github.com/getkin/kin-openapi` v0.149.0；`golang.org/x/crypto` v0.57.0；`golang.org/x/time` v0.16.0；`golang.org/x/text` v0.42.0；`github.com/testcontainers/testcontainers-go` 與 `modules/postgres` v0.44.0。間接依賴由 MVS 決定，`go mod tidy` 後若被拉高到冷卻期內的版本，回報。
  - Go 工具：`github.com/oapi-codegen/oapi-codegen/v2` v2.8.0、`github.com/sqlc-dev/sqlc` v1.31.1、`github.com/golangci/golangci-lint/v2` v2.13.2（v2.13.0 起支援 go1.27）、`golang.org/x/vuln` v1.8.0、`github.com/oasdiff/oasdiff` v1.33.0、`github.com/boumenot/gocover-cobertura` v1.5.0。
  - Atlas v1.1.3（D12）：`https://release.ariga.io/atlas/atlas{,-community}-<os>-<arch>-v1.1.3`，sha256 取自同名 `.sha256`：community linux-amd64 `db596f9e39c33aae3bcbed674dab2255dd20b5e7bee29f7ed83413f6dd7b5047`、linux-arm64 `511996bddf663d14a243d3e95ebb0d3d1d0a2aa89abb2fc85810a12cad7c390f`；standard linux-amd64 `d2c3675dcf5f88989506e34b3de4d8b76f7fb23d02422c4b5e5dd59fe257f1ab`、linux-arm64 `6c898b2aaa0e84e84ecbf4d8f9b74007f16815759069651a718e9bee25319656`、darwin-arm64 `b587a8d1c4da7f5891300e06576b0286fd0270b7c531026f019db1558d0706d0`。
  - 其他工具：squawk-cli 2.65.0（npm）、diff-cover 10.5.1（PyPI）。
  - 前端：openapi-typescript 7.13.0（devDependency）、openapi-fetch 0.17.0；swagger-ui-dist 5.33.0（jsDelivr，只在 dev）。
  - GitHub Actions：`actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0`。
  - toolchain 與 image（D13 的安全修補例外）：Go 1.27.2；`golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e`；`gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3`；`caddy:2.11.7@sha256:685ae3d7c34d9b5f8caeddc5d14051db7bbd88ff1f5a2fa0c16bd8ca328fb7af`。Debian 用 13（trixie），和 FathomPod 一致（預設，可推翻）。
  - 密碼清單：SecLists `Passwords/Common-Credentials/Pwdb_top-1000000.txt`，commit `12274c98fdebe98c7a7284914436a472ed469aed`，sha256 `e9a88f67aafe65496682dc374559ee714e978bee50314767494c3e37a18c9fc8`，MIT（SecLists 與上游 ignis-sec/Pwdb-Public）。只內嵌長度 ≥15 code point 的 5,406 筆（實測）。NCSC 100k 不收：原檔授權未載明，且 ≥15 的只有 331 筆。

**驗收基線（舊系統，`7a4d0fa`，2026-10-18，本機）：**
- 後端：`uv run pytest -q` → `30 passed in 3.47s`（PostgreSQL 18 容器）。
- 前端：`pnpm test` → `Test Files 3 passed (3)、Tests 17 passed (17)`；`pnpm lint` → `Checked 39 files … No fixes applied.`
- 既有失敗：無。
- 新 Go 後端：尚未建立，基線在步驟 1 建立。

## 本階段依賴的不變量

| 不變量 | 依賴它的機制 |
|---|---|
| **I1　契約唯一來源。** `backend/api/openapi.yaml` 是 API 形狀、長度與格式限制、enum 的唯一定義。後端生成碼與前端生成型別都必須和它一致 | <ul><li>oapi-codegen 生成到 `internal/httpapi/gen`</li><li>validator middleware 在 `internal/httpapi/validate.go`</li><li>openapi-typescript 生成到 `frontend/src/lib/api/schema.d.ts`</li><li>CI 的「重新生成後 diff 必須為空」檢查（前後端各一）</li><li>oasdiff（合併後生效）</li><li>本次改寫由 `parity.sh` 檢查舊到新的對照</li></ul> |
| **I2　錯誤一律是 RFC 9457 problem。** 每個 problem 有固定的 `type` URI；驗證錯誤帶 `errors[{pointer, code, detail}]`；前端只依 `type`、`code` 顯示訊息，不解析 `detail` | <ul><li>`internal/httpapi/problem.go`</li><li>`internal/platform/apperr`（kind 與 code）</li><li>`frontend/src/lib/api/errors.ts`</li><li>`frontend/messages/*.json` 的 `problems.*` 鍵</li></ul> |
| **I3　認證憑證只在 `HttpOnly` cookie，JS 讀不到。** 所有狀態變更請求都經過 `CrossOriginProtection` | <ul><li>`internal/httpapi/auth.go`（cookie 的寫入與清除）</li><li>`internal/httpapi/server.go`（`CrossOriginProtection` 包住整個 mux）</li><li>`frontend/src/proxy.ts`（只檢查 cookie 是否存在）</li><li>`features/auth/server.ts`（轉發 cookie）</li></ul> |
| **I4　分層與依賴方向**（見表後說明） | <ul><li>`.golangci.yml` 的 depguard 規則</li><li>`internal/httpapi` 負責組裝</li><li>`cmd/app`</li></ul> |
| **I5　schema 只由 Atlas 產生。** Atlas standard 版從各模組的 `schema.sql` 產生 migration，auth 的 `schema.sql` 先載入；baseline 之後的 migration 只增不改；image 內用 community 版套用（D12） | <ul><li>`backend/atlas.hcl`（`src` 順序：auth、items）</li><li>`backend/Dockerfile` 的 community 版與 CI、`backend/Makefile` 的 standard 版，版本與 checksum 照 P14</li><li>`backend/migrations/`</li><li>`backend/scripts/check-schema-drift.sh`、`backend/scripts/check-migrations-immutable.sh`（寫法照 FathomPod 同名腳本）</li><li>root `AGENTS.md` 的 Constraints</li></ul> |
| **I6　授權在查詢層。** 使用者擁有的資源，查詢一律帶 `owner_id = $current`，非 owner 回 404；admin 操作在 handler 前由 `requireRole("admin")` 把關 | <ul><li>`internal/modules/items/queries/*.sql`</li><li>`internal/httpapi/authz.go`</li><li>測試 `TestGetOtherUsersItem`、`TestNonAdminCannotPatchUser`</li></ul> |

**I4 的具體規則：**
- HTTP handler、cookie、生成碼放在 `internal/httpapi`。
- `internal/modules/<m>` 只放 service、store 和 sqlc 生成碼，不 import `net/http`，也不 import 其他模組。
- 只允許一個跨模組 FK：`items.owner_id → users.id`。items 的 `sqlc.yaml` 的 `schema` 同時列出 auth 與 items 的 `schema.sql`，但 items 的查詢不得 `SELECT` 或 `JOIN` `users`。
- 目前使用者經由 `internal/platform/authctx`（只用 `context.Context`）傳給模組。

## 規模（搬遷）

| | auth | items | 共用（core/health） | 產生計數的位置 |
|---|---|---|---|---|
| REST operation（新） | 6 | 5 | 2 | 盤點 A 段 |
| 表（新） | 3（`users`、`sessions`、`login_throttles`） | 1 | 0 | 盤點 C 段 |
| 舊測試 | 16 | 8 | 1＋5（config） | `test-map.md` |
| 既有缺陷 | E2、E3、E5、E8、E10、E11、E12、E13、E14 | E1、E6、E9 | E4、E7、E15 | 盤點 E 段 |

## 範圍

- **做：** 盤點 A–F 段的全部列；E 段 15 項缺陷全修。專案頁 Pending 已在 2026-10-18 依 D5 處置。
- **不做：**
  - 背景工作框架（River）：沒有非同步工作。
  - OAuth、SSO、MFA、email、找回密碼：範本的認證先用 username 加密碼，觸發點見延後項。
- **延後（附觸發條件）：**
  - email 驗證與找回密碼（需要寄信服務）。觸發：第一個需要找回密碼的專案。幾乎每個接案專案都會碰到，ADR 已寫明。
  - MFA，或改用代管認證。觸發：需要 MFA、SSO 或第三方登入。
  - OpenTelemetry tracing。觸發：第一個接上 tracing 後端的部署。
  - 背景工作（River）。觸發：出現第一個非同步工作。
  - 多實例的 rate limit 改用 DB 或 proxy 層。觸發：backend 跑超過一個 replica。
  - OpenAPI 3.1。觸發：oapi-codegen 宣布 3.1 支援穩定。

## 步驟

- [x] **0. 查證與依賴核准**（被擋於：無）
  - 範圍：
    - 查證 P13 的事實，把結論寫回前提段。
    - 列出依賴清單交 Will 核准。每項附確切版本、發布日期，以及 deps.dev（或 GHCR、Docker Hub、npm）連結；30 天內才發布的版本不採用。
      - Go module：
        - `github.com/jackc/pgx/v5`
        - `github.com/google/uuid`（oapi-codegen runtime 會帶入，全專案共用，見 P4）
        - `github.com/oapi-codegen/runtime`、`github.com/oapi-codegen/nullable`、`github.com/oapi-codegen/nethttp-middleware`
        - `github.com/getkin/kin-openapi`
        - `golang.org/x/crypto`（argon2）、`golang.org/x/time`（rate）
        - `golang.org/x/text`（NFKC，見 P7）
        - `github.com/testcontainers/testcontainers-go` 與 `modules/postgres`
      - Go 工具（用 `go tool` 或在 CI 安裝並鎖版本）：oapi-codegen、sqlc、Atlas CLI、golangci-lint v2、govulncheck、oasdiff、`github.com/boumenot/gocover-cobertura`。
      - 其他工具：squawk-cli（npm，`npx --yes squawk-cli@<ver>`）、diff-cover（PyPI，CI 用 `pipx run diff-cover==<ver>`）。
      - 前端：`openapi-typescript`（devDependency）、`openapi-fetch`。
      - GitHub Actions：`actions/setup-go`，鎖 SHA（zizmor 會檢查）。
      - Docker image：`golang:1.27-trixie`（builder）、`gcr.io/distroless/static-debian13`（runtime）、`caddy:2`（正式 compose），都鎖 digest；Atlas 二進位的下載 URL 與 checksum。（結果見 P14。）
      - 資料：常見與外洩密碼清單，附來源、授權、筆數（候選：NCSC 的 PwnedPasswordsTop100k、SecLists）。
      - dev 文件頁：`swagger-ui-dist`，從 jsDelivr 載入並鎖版本，只在 `MODE=dev` 提供。
  - 消費端：無（這一步只產出清單）。
  - 不能動：任何檔案，計畫的前提段除外。
  - 驗收：
    - P13 每一項都寫上「已確認」並附來源 URL，或寫上「不成立」並說明改法。
    - 清單每一列都有版本、發布日期、連結。
    - Will 回覆核准。
  - 停止條件：
    - 某個套件找不到符合條件的版本 → 回報替代方案。
    - Atlas community 版缺少必要功能 → 回報；換成非 community 版之前先問。
  - 需要人做的事：Will 核准清單。
- [ ] **1. Walking skeleton**（被擋於：步驟 0）
  - 範圍：
    - 分支與保存：開 `go-rewrite` 分支；在開工時的 `main` 打 tag `archive/pre-go-rewrite` 並 push；開 draft PR。
    - 刪除 Python 後端：`backend/src`、`tests`、`alembic*`、`pyproject.toml`、`uv.lock`、`scripts/wait_for_db.py`、`.python-version`（若有）。
    - 建立 Go 骨架：
      - `go.mod`；`cmd/app`，子指令只有 `serve`。
      - `internal/platform/config`（盤點 D 段、E10）、`internal/platform/logx`（slog）。
      - `internal/httpapi/server.go`：盤點 B19 的 timeout、body 上限、graceful shutdown；`CrossOriginProtection`（B21）。
      - `GET /health`、`GET /health/ready`（E7）。
    - 建立 Atlas 設定：`atlas.hcl`、空的 `migrations/`。
    - 建置與部署：
      - `backend/Dockerfile`：multi-stage，內附 Atlas community CLI（P14 的 URL 與 checksum，`sha256sum -c` 驗證）。
      - `docker-compose*.yml`（F5）。
      - `backend/Makefile`：`check` 先包含 gofmt、golangci-lint、`go test -race`、govulncheck。
      - root `Makefile` 的 `migration`、`generate-migration`、`dev`（F4 前半）。
    - 在 root `AGENTS.md`、`backend/AGENTS.md`、`docs/rules/*` 開頭加上改寫期間的提示行（見全域約束）。
      - `.env-example`（F6 的 backend 部分）。
    - CI 與 lint：
      - CI 的 Go job（F1 的 lint、test、govulncheck 部分）；安裝 Atlas standard 版並驗 checksum，供步驟 2 的漂移檢查使用。
      - `.golangci.yml`：含 I4 的 depguard 規則與 gosec。
  - 消費端：`rg -n "backend|8000" docker-compose.yml docker-compose.dev.yml Makefile .github/workflows/ci.yml .env-example`
  - 不能動：
    - `frontend/`；
    - root `Makefile` 的 `seed`、`reset-db`：步驟 3 才改，這段期間暫時不能用。
  - 驗收：
    - `make -C backend check` 綠燈。
    - `docker compose up -d --build` 後：
      - `curl -fsS localhost:8000/health` 回 `{"status":"ok"}`；
      - `curl -fsS localhost:8000/health/ready` 回 `{"status":"ready"}`；
      - migrate service 以 exit 0 結束；
      - `docker compose port backend 8000` 顯示 `127.0.0.1:8000`。
    - `docker compose stop postgres` 後，`/health/ready` 回 503。這一步只斷言 status code，problem 格式在步驟 2 驗收。
      - `TestReadyDBDown` 用注入的 fake pinger，不需要 DB。
      - mutation：把 503 改回 500，`TestReadyDBDown` 要失敗。
    - 送出 SIGTERM 後，`/health/ready` 在 shutdown 期間回 503（`TestReadyDuringShutdown`）。
    - `TestBodyTooLarge` 回 413。
      - mutation：拿掉 `MaxBytesReader`，測試要失敗。
    - `TestSlowHeaderClosed`：測試把 `ReadHeaderTimeout` 設成 100ms，慢慢送 header，連線要被關閉。
      - mutation：拿掉 `ReadHeaderTimeout`，測試要失敗。
    - `TestCrossOriginPostRejected`：用一個暫時的 POST handler 測試，步驟 3 再換成真正的 handler。
      - mutation：拿掉 `CrossOriginProtection`，測試要失敗。
    - config 測試：照 `test-map.md` 的前 5 列。
    - draft PR 的 `gh pr checks` 全綠。
    - 擋點：compose 的驗收要等 Will 回覆「`.env` 已依 P12 更新」才跑；`make -C backend check` 不需要等。
  - 停止條件：
    - `atlas migrate apply` 對空目錄以非 0 結束 → migrate service 先直接 `exit 0`，步驟 3 補上，並記在進度表。
    - Atlas CLI 無法在 distroless 執行 → 回報；改用 Debian slim 之前先問。
  - 需要人做的事：Will 依 P12 更新本機 `.env`。
  - 完成後的狀態：前端登入暫時不能用。步驟 1–5 全部完成才合併。
- [ ] **2. 共用平台**（被擋於：步驟 1）
  - 範圍：
    - 契約與生成：
      - `api/openapi.yaml` 骨架，含共用 schema `Problem`、`ValidationProblem`，以及 cursor 分頁參數。
      - oapi-codegen 生成到 `internal/httpapi/gen`。
      - validator middleware，負責把 kin-openapi 的錯誤轉成 `errors[{pointer, code, detail}]`。code 對照：`required`、`too_short`、`too_long`、`pattern`、`enum`、`format`、`out_of_range`、`invalid_type`、`null_not_allowed`、`additional_property`；其餘一律用 fallback `invalid`。Content-Type 不符時另外回 415，problem `type` 為 `/problems/unsupported-media-type`。
    - 錯誤處理：
      - `internal/platform/apperr`。
      - `internal/httpapi/problem.go`：`type` URI 清單見盤點 B2；未處理的錯誤回 500 並記 log；含 panic recover。
    - middleware：
      - request id（E5）。
      - 安全標頭（B4、E4）。
      - access log，記 `duration_ms`。
      - per-IP rate limiter（B6、E3）。
    - 分頁：`internal/platform/httpx` 的 cursor 編碼與解碼（B9）。
    - 資料層工具：sqlc、squawk、兩支 schema 腳本（I5）、`concurrent_index` policy。
    - 測試 harness：testcontainers 起 PG 18，套用 `migrations/`。
    - dev 文件：`GET /api/openapi.yaml` 與 Swagger UI。
    - `make check` 加入：
      - sqlc diff；
      - 漂移檢查；
      - squawk；
      - migrations-immutable；
      - 「重新生成 `internal/httpapi/gen` 後 diff 必須為空」。
    - CI 加入：
      - oasdiff（`main` 上沒有 `openapi.yaml` 時跳過）；
      - coverage → Cobertura → diff-cover。
  - 消費端：無外部消費端（模組在步驟 3、4 才接上）。
  - 不能動：I2 的欄位名。
  - 驗收（各條測試對應 `test-map.md` 的新增列；每個 mutation 都必須讓測試失敗）：
    - 測試方式：validator、rate limiter 與 problem 都用 `internal/httpapi/testdata/test-openapi.yaml` 加 stub handler 驗收，因為真正的 operation 要到步驟 3、4 才有。
    - 違反 `minLength` 的 request 回 422，且 `errors[0]` 有 `pointer` 與 `code: too_short`。
      - mutation：把 code 對照表的 `minLength` 改成回 fallback `invalid`，測試要失敗。
    - Content-Type 不符回 415，problem `type` 為 `/problems/unsupported-media-type`。
    - `GET /health` 經過完整 middleware 鏈仍回 200；dev 模式的 `GET /api/openapi.yaml` 回 200（不被 validator 擋下）。
    - 不合法的 request id 會被換掉；不受信任的來源送的 request id 不被採用。
      - mutation：拿掉格式檢查。
    - 回應帶 `Cache-Control: no-store`，沒有 `X-XSS-Protection`。
      - mutation：拿掉 `no-store`，`TestSecurityHeaders` 要失敗。
    - per-IP rate limit：
      - 來源在 `TRUSTED_PROXIES` 內時，兩個不同的 XFF 各自計數；
      - 來源不受信任時忽略 XFF；
      - 超限回 429，帶 `Retry-After`。
      - mutation：改成一律採用 XFF。
    - handler panic 時回 500 problem。
      - mutation：拿掉 recover，`TestPanicReturnsProblem` 要失敗。
    - depguard：在 `internal/modules/` 暫放一個 import `net/http` 的檔案，`golangci-lint run` 要失敗；確認後刪除該檔。
    - `make -C backend check` 綠燈、PR CI 綠燈。
- [ ] **3. auth 模組**（被擋於：步驟 2）
  - 範圍：
    - `internal/modules/auth`：
      - `schema.sql`：盤點 C1、C2、C4。
      - queries，經 sqlc 生成。
      - service：
        - 建立使用者：建 user 與建 session 在同一個 transaction。
        - 登入：依帳號節流，含定期清理超過 24 小時的 `login_throttles` 列；`serve` 每小時執行一次。
        - 登出（目前 session）、登出全部 session、目前使用者、admin 停用使用者並撤銷 session。
        - 依據：盤點 B6、B10、B12、B15、B17、B18、B20，前提 P6、P7、P8。
      - `blocklist/`：內嵌的密碼清單，以及 HIBP 選項。
    - `internal/httpapi`：
      - `auth.go`、`users.go`：盤點 A3、A4、A6、A7、A12。
      - `authz.go`：`requireRole`，對應 I6。
    - `internal/platform/authctx`。
    - 用 `atlas migrate diff baseline` 產生 baseline migration。
    - `cmd/app seed-admin`（E8）：從內嵌的 `openapi.yaml` 取 `CreateUserRequest` schema，用 kin-openapi 驗證輸入（I1）；帳號已存在時不做任何修改，以 exit 0 結束。
    - root `Makefile` 的 `seed`、`reset-db`（F4 後半）。
    - 若步驟 1 讓 migrate service 直接 `exit 0`，這一步補上真正的 migrate。
  - 消費端：`rg -n "/auth/" frontend/src`，前端在步驟 5 改。
  - 不能動：
    - I1–I3、I6。
    - timing 防護：未知帳號也要跑一次 argon2 驗證。
    - 節流行為不得洩漏帳號是否存在。
  - 驗收：
    - `test-map.md` 的 auth 16 列與 auth 新增列全部通過。
    - mutation，每一項都必須讓指定的測試失敗：
      - (a) 拿掉 dummy hash 分支 → `TestCreateSessionUnknownAccountRunsHash`。
      - (b) 登入時沿用請求帶來的 token → `TestCreateSessionIssuesNewToken`。
      - (c) 拿掉 idle 期限檢查 → `TestSessionIdleExpiry`。
      - (d) 拿掉 `is_disabled` 檢查 → `TestCreateSessionDisabledAccount`。
      - (e) 只對存在的帳號節流 → `TestLoginThrottlePerAccount`。
      - (f) 停用帳號時不撤銷 session → `TestAdminDisablesUserRevokesSessions`。
      - (g) 拿掉清單比對 → `TestCreateUserCommonPassword`。
      - (h) 拿掉 argon2 semaphore → `TestArgonConcurrencyBounded`。
      - (i) `seed-admin` 不驗證輸入 → `TestSeedAdminRejectsShortPassword`。
      - (j) 拿掉 NFKC → `TestPasswordNormalized`（用全形與半形版本的同一組密碼，要能互相登入）。
    - `TestSeedAdminExistingAccountUnchanged`：既有的 `user` 帳號跑 `seed-admin` 後，role 不變。
    - 用 curl 手動走一遍（帶 `Origin` 與 `Sec-Fetch-Site: same-origin`）：
      1. `POST /api/users` 回 201；
      2. `GET /api/users/me` 回 200；
      3. `DELETE /api/sessions/current` 回 204；
      4. 再打 `GET /api/users/me` 回 401。
    - PR CI 綠燈。
- [ ] **4. items 模組**（被擋於：步驟 3）
  - 範圍：
    - `internal/modules/items`：
      - `schema.sql`：盤點 C3。
      - `sqlc.yaml`：依 I4。
      - queries 與 service：查詢一律帶 owner 條件（I6）。
    - `internal/httpapi/items.go`：盤點 A8–A11。
    - 用 `atlas migrate diff` 產生第二支 migration。
    - 重新生成前端的 `frontend/src/lib/api/schema.d.ts`（`pnpm -C frontend gen:api`），讓前端 CI 的「重新生成後 diff 必須為空」保持綠燈。
  - 消費端：`rg -n "/items" frontend/src`，預期只命中 `lib/__tests__/api-client.test.ts`。
  - 不能動：I1、I2、I6。
  - 驗收：
    - `test-map.md` 的 items 8 列，以及 `TestCreateItemTitleTooLong`、`TestUpdatedAtChanges` 全部通過。
    - mutation，每一項都必須讓指定的測試失敗：
      - (a) 拿掉 owner 條件 → `TestGetOtherUsersItem`。
      - (b) 把 PATCH 的 request 型別改成一般指標 → `TestUpdateItemMergePatch`。
      - (c) keyset 條件改成 offset → `TestPaginationKeyset`（測試會在分頁途中插入新資料）。
      - (d) update 查詢拿掉 `updated_at = now()` → `TestUpdatedAtChanges`。
    - `rg -n "users" backend/internal/modules/items/queries` 沒有結果（I4）。
    - PR CI 綠燈。
  - 完成後的狀態：後端完成；前端仍是舊的呼叫方式，必須接著做步驟 5。
- [ ] **5. 前端切換與同源部署**（被擋於：步驟 4）
  - 範圍：
    - 盤點 F16–F22、F25–F27，以及 F6 的前端部分。
    - 同源部署：`deploy/Caddyfile`，以及正式 compose 的 `caddy` service、固定 subnet、`TRUSTED_PROXIES`（P10、F5 的正式環境部分）。部署文件（F10）在步驟 6 改寫。
    - 用 `pnpm gen:api` 從 `../backend/api/openapi.yaml` 生成 `src/lib/api/schema.d.ts`。
    - `src/lib/api/client.ts`：openapi-fetch，browser 端 baseUrl 是 `/api`，server 端是 `API_URL` 加上 `/api`。
    - `src/lib/api/errors.ts`：把 problem 轉成 next-intl 的訊息鍵。
    - `next.config.ts` 的 `rewrites`。
    - `proxy.ts` 的 CSP 改成 `connect-src 'self'`。
  - 消費端：
    - `rg -n "refresh|access_token|refresh_token|URLSearchParams|\.data\b|/auth/" frontend/src frontend/AGENTS.md frontend/README.md`
    - `rg -n NEXT_PUBLIC_API_URL . -g '!docs/adr/**' -g '!docs/plans/**' -g '!frontend/node_modules/**'`：步驟 5 完成時必須沒有結果
  - 不能動：
    - I1–I3；
    - `getServerUser()` 仍然是真正的認證關卡（`frontend/AGENTS.md`）。
  - 驗收：
    - `pnpm lint`、`pnpm test`、`pnpm build` 都綠燈。
    - CI 新增一項：重新生成 `schema.d.ts` 後 diff 必須為空。
      - mutation：在 `openapi.yaml` 改一個欄位但不重新生成，CI 要失敗。
    - 新測試：
      - 收到 401 時直接導向 login，不呼叫 `/auth/refresh`；
      - problem 的 `code` 會轉成翻譯後的訊息；
      - `proxy.ts` 只在有 session cookie 時放行受保護的路徑。
      - mutation：在 `errors.ts` 改用 `detail` 顯示，翻譯測試要失敗。
    - 正式 stack：`make build && make up`（`SITE_ADDRESS=:80`、`COOKIE_SECURE=false`）後：
      - `curl -s -o /dev/null -w '%{http_code}' http://localhost/api/users/me` 回 `401`；
      - 打一次 `POST http://localhost/api/sessions`，backend access log 的 `client_ip` 是 Caddy access log 的 `remote_ip`，不是 Caddy 容器自己的 IP（證明 `TRUSTED_PROXIES` 生效，E3 不會回來）；
      - `docker compose ps` 顯示只有 caddy 發佈 port。
    - 端到端：在上述正式 stack 上用瀏覽器（Claude in Chrome）錄 GIF，走完：
      1. 註冊；
      2. 進入 overview；
      3. 重新整理後仍是登入狀態，證明 SSR 讀得到 cookie（E11）；
      4. 登出；
      5. 被導回 login。
    - PR CI 綠燈。
- [ ] **6. 外圍與文件**（被擋於：步驟 4、5）
  - 範圍：
    - 盤點 F2、F3、F7–F15、F23、F24；移除步驟 1 加的改寫期間提示行。
    - `rename.py`：module path 規則照 P4（含可選的 `MODULE=`），以及 F12。
    - root `AGENTS.md`：
      - Verification 改為 `make -C backend test` 與 `pnpm test`；
      - Constraints 的 migration 規則改寫成 I5；
      - 依賴規則保留。
    - `docs/rules/*`：照 F9 改寫。
    - `.claude/skills/add-module.md`、`.claude/skills/add-crud.md`：改寫成 Go 的骨架。
  - 消費端：`python3 -I docs/plans/2026-10-18-go-backend-rewrite/check_inventory.py --after-rewrite`
  - 不能動：`make rename` 的 `NAME=` 介面。
  - 驗收：
    - 在 scratchpad 的 clone 執行 `make rename NAME=acme` 後：
      - `head -1 backend/go.mod` 等於 `module github.com/Will413028/acme/backend`；
      - `rg -n "fullstack-template/backend" backend` 沒有結果；
      - `go build ./...`、`go test ./...`、`pnpm build` 都綠燈。
    - 另跑一次 `MODULE=example.com/x/backend`，`go.mod` 第一行要跟著改。
    - mutation：讓 rename 不改 import，`go build` 要失敗。
    - `check_inventory.py --after-rewrite` 輸出 `OK`。
- [ ] **7. 對照驗證**（被擋於：步驟 4、5）
  - 範圍：寫 `docs/plans/2026-10-18-go-backend-rewrite/parity.sh`。契約已經刻意改變，所以這一步不比對形狀是否相同，而是驗證**舊系統的每一個業務行為，在新系統都有對應的結果**。
    - 環境：
      - 兩邊各用一個獨立的 PG 18 容器。
      - 舊系統：在 tag worktree 執行 `uv sync`、`alembic upgrade head`，再用 `python -c "from src.auth.router import limiter; limiter.enabled=False; import uvicorn; from src.main import app; uvicorn.run(app, port=8001)"` 啟動（關掉 rate limit 的作法同 `tests/conftest.py:91-93`）。環境變數：`DATABASE_URL`、`SECRET_KEY`（測試值）、`COOKIE_SECURE=false`。
      - 新系統：`AUTH_RATE_LIMIT_PER_MIN=10000`、`COOKIE_SECURE=false`。
    - 對照表：每一列寫「舊請求與預期結果 → 新請求與預期結果」，涵蓋：
      - 註冊、重複註冊、弱密碼（各依新規則）；
      - 登入成功與失敗、取得目前使用者；
      - item 的建立、列出、分頁、讀取、修改、刪除；
      - 讀取別人的 item、未登入。
    - 比對內容：
      - 兩邊的業務結果相等：成功或失敗、資料內容（title、description、owner 是否相同）、哪些 item 可見。
      - status 與形狀依對照表各自斷言。
  - 消費端：無。
  - 驗收：
    - `parity.sh` 全部通過。
    - mutation：讓新系統的 `GET /api/items` 漏掉最後一筆，`parity.sh` 要失敗。
- [ ] **8. 收尾**（被擋於：步驟 1–7）
  - 完成定義逐項核對。
  - 跑 design-review skill（獨立 subagent）。
  - `check_inventory.py --after-rewrite` 輸出 `OK`。
  - 長期檢查都已接進 CI：depguard、漂移、migrations-immutable、前後端重新生成 diff、oasdiff。
  - code review。
  - PR 轉成 ready，CI 綠燈後合併（範本沒有部署，合併不算 deploy）。
  - 本計畫檔與附件目錄在 PR 的最後一個 commit 刪除，避免每個下游 clone 都帶走它。進度摘要寫進 PR 內文與專案頁。ADR 保留。
  - 延後項連同觸發條件搬回專案頁 `## Pending`，並刪除本計畫在 Pending 的掛載項。
  - 盤點 F28：更新 `AGENTS.local.md`（刪除計畫指向）；專案頁的 frontmatter `tech`／`tags`、Architecture、Key Files 改成 Go 版；Pending 中以「Go 改寫合併後」為觸發的兩項（osv-scanner 重跑、Go 端鎖定測試）標為可開工。
  - 本機 `.claude/settings.local.json` 的 Python 指令 allowlist（git 不追蹤）列給 Will 自行決定是否清理。
  - 驗收：
    - `gh pr checks` 全綠的輸出；
    - design-review 的每一項發現都有處置。

## 決定

- **D1 範圍**（已決 2026-10-18，Will；擋住：1）
  - 直接取代：刪除 Python，舊系統打 tag 保存。
  - ADR：`docs/adr/2026-10-18-go-backend.md`。
- **D2 對外契約**（已決 2026-10-18，Will；擋住：2–5）
  - REST＋OpenAPI spec-first，形狀照業界基準（盤點 A、B 段）。2026-10-18 第二次拍板「照建議」時，撤回第一版「保留現有路徑與外殼」的寫法。
  - ADR：`docs/adr/2026-10-18-api-and-auth-baseline.md`。
- **D3 資料層與 migration**（已決 2026-10-18，Will；擋住：2–4）
  - pgx＋sqlc＋Atlas 宣告式。
  - ADR：`docs/adr/2026-10-18-go-backend.md`。
- **D4 認證**（已決 2026-10-18，Will；擋住：3、5）
  - DB opaque session，細節見 P6–P8。
  - ADR：`docs/adr/2026-10-18-api-and-auth-baseline.md`。
- **D5 專案頁 Pending 的 Python 後端項目**（已決 2026-10-18，Will）
  - 移除；同日已在專案頁逐行處置。
- **D6 DB 型別規則**（已決 2026-10-18，Will 照建議）
  - 照 PG wiki「Don't Do This」：`text` 加 CHECK、狀態欄 CHECK、UUIDv7 主鍵、`timestamptz`。
  - ADR：`docs/adr/2026-10-18-go-backend.md`。
- **D7 主鍵**（已決 2026-10-18，Will 照建議）
  - 用 UUIDv7，不用 bigint 轉字串。
- **D8 分頁**（已決 2026-10-18，Will 照建議）
  - 範本預設 cursor（keyset），需要頁碼的後台表格可選用 offset。
- **D9 密碼清單**（已決 2026-10-18，Will 照建議）
  - 內嵌清單；HIBP 為可選設定。
- **D10 登入識別**（已決 2026-10-18，Will 照建議）
  - username 不分大小寫；email 與找回密碼延後。
- **D11 role**（已決 2026-10-18，Will 照建議）
  - 保留 `admin`／`user`，並提供一個 admin 操作：停用使用者。

- **D12 Atlas 版本別**（已決 2026-10-18，Will 照建議；擋住：1–4）
  - 產生 migration 與漂移檢查用 standard v1.1.3（EULA、免費、不需登入），image 內套用用 community v1.1.3；不選「全用 community 加自寫合併腳本」。依據 P13。
  - ADR：`docs/adr/2026-10-18-go-backend.md`（D2 修訂）。
- **D13 冷卻期的安全修補例外**（已決 2026-10-18，Will 照建議）
  - 寫在全域約束「依賴」。依據：Dependabot 的 cooldown 不擋安全更新；Go 1.27.2 修 `net/http`、`crypto/tls`，Caddy 2.11.4 有 GHSA-8qx7-j2v2-x6q5（≥2.11.5 修掉）。

拍板順序：全部已決。P4、P9、P11 是預設，Will 可推翻；推翻時依 phase-plan 步驟 5 重寫受影響的步驟。

## 進度

狀態值：未開始／進行中／程式完成待驗收／完成。

| 步驟 | 狀態 | 已跑的驗收 | 未跑的驗收與原因 | commit |
|---|---|---|---|---|
| 0 查證與依賴核准 | 完成 | P13 五項已查並寫回；清單 Will 2026-10-18 核准（P14）；D12、D13 已決 | 無 | `4ae2473` 與本 commit |
| 1 Walking skeleton | 未開始 | | | |
| 2 共用平台 | 未開始 | | | |
| 3 auth | 未開始 | | | |
| 4 items | 未開始 | | | |
| 5 前端切換 | 未開始 | | | |
| 6 外圍與文件 | 未開始 | | | |
| 7 對照驗證 | 未開始 | | | |
| 8 收尾 | 未開始 | | | |
