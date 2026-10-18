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
  - `DATABASE_URL` 去掉 `+asyncpg`，並加上 `?sslmode=disable`（P13）；
  - 刪除 `SECRET_KEY`、`COOKIE_SAMESITE`、`COOKIE_DOMAIN`、`CORS_ORIGINS`、`NEXT_PUBLIC_API_URL`。
- **P13　步驟 0 查證的事實**（2026-10-18）：
  - **`http.Server` timeout 與 gosec G112：已確認。** 欄位有 `ReadTimeout`、`ReadHeaderTimeout`、`WriteTimeout`、`IdleTimeout`、`MaxHeaderBytes`。`ReadHeaderTimeout` 為 0 時沿用 `ReadTimeout`，兩者都是 0 就沒有 timeout；`IdleTimeout` 同理（[go1.27.2 `src/net/http/server.go:3089-3118`](https://raw.githubusercontent.com/golang/go/go1.27.2/src/net/http/server.go)）。G112 只檢查 `http.Server{...}` literal 裡有沒有寫 `ReadHeaderTimeout` 或 `ReadTimeout` 這個 key，不看值，事後賦值（`srv.ReadHeaderTimeout = …`）也不算（[gosec `rules/slowloris.go`](https://github.com/securego/gosec/blob/master/rules/slowloris.go)）。所以 G112 擋不住設成 0；B19 的 timeout 要靠 `TestSlowHeaderClosed` 守住，不能靠 lint。
  - **Docker 發佈的 port 繞過 ufw：已確認。** Docker 文件：發佈 port 後，封包「are routed before the firewall rules can be applied, effectively ignoring your firewall configuration」（[Packet filtering and firewalls](https://docs.docker.com/engine/network/packet-filtering-firewalls/)）。P10 的設計因此不變：只有 Caddy 發佈 port，dev compose 綁 `127.0.0.1`。
  - **Atlas community 版：不成立，已由 D12 處理。** 下載 URL `https://release.ariga.io/atlas/atlas-community-<os>-<arch>-<ver>` 確認是 community 版。但 FathomPod 在 v1.1.3 實測過：community 版不接受多個 schema 檔的 `src`，`concurrent_index` 產的檔也不加 `-- atlas:txmode none`（wiki `tech/atlas-migrate-ci-gate` Gotcha 6、Case 25；FathomPod ADR `2026-10-07-go-rewrite-scope-and-cutover`）。所以 FathomPod 改成產生 migration 與漂移檢查用 standard 版、image 內套用用 community 版（`backend/Dockerfile:15-27`、`.github/workflows/ci.yml:93-97`）。這和 I5 的「各模組一份 `schema.sql`」直接衝突，觸發步驟 0 的停止條件；Will 選擇照 FathomPod 分兩個版本別（D12）。
  - **Atlas 與 pgx 的 `sslmode` 預設：不同。** Atlas v1.3.0 用 `lib/pq` v1.12.0（`cmd/atlas/go.mod:20`），它的預設是 `require`（`connector.go:61`）；pgx v5.11.0 照 libpq 預設 `prefer`（`pgconn/config.go:296-302`）。compose 內的 PostgreSQL 沒開 TLS，所以 Atlas 不帶參數會連不上。改法：`.env-example` 的 `DATABASE_URL` 直接寫 `?sslmode=disable`（FathomPod `backend/deploy/compose.yaml:102,117` 也帶這個參數）。步驟 1 實測：community v1.1.3 不帶參數時回 `pq: SSL is not enabled on the server`。FathomPod 的 migrate 與 serve 用不同的 DB 角色；本範本照 D15 分成 owner 與 app，密碼改由 `*_FILE` 傳入，不在 URL 裡。
  - **`http://localhost` 上的 `Secure` cookie：已確認，不影響設計。** Chrome 與 Firefox 接受；Safari 拒收 `Secure`、`__Host-`、`__Secure-`（[WebKit bug 231035](https://bugs.webkit.org/show_bug.cgi?id=231035)、[httpwg issue 2605](https://github.com/httpwg/http-extensions/issues/2605)，屬推論：bug 報告是 2021–2024 年的，沒有在當前 Safari 實測）。`COOKIE_SECURE=false` 時 cookie 改名為 `session`、不帶 `Secure`，三家都能用。

- **P14　核准的依賴**（Will 2026-10-18 核准；查證日 2026-10-18，冷卻期截止 2026-09-18；OSV 查過，只有 x/crypto 的 GO-2026-5932，影響 `openpgp`，本專案不用）。清單外的套件或換版本仍照全域約束「依賴」：
  - Go module：`github.com/jackc/pgx/v5` v5.11.0；`github.com/google/uuid` v1.6.0；`github.com/oapi-codegen/runtime` v1.7.0；`github.com/oapi-codegen/nullable` v1.2.0；`github.com/oapi-codegen/nethttp-middleware` v1.2.0；`github.com/getkin/kin-openapi` v0.149.0；`golang.org/x/crypto` v0.57.0；`golang.org/x/time` v0.16.0；`golang.org/x/text` v0.42.0；`github.com/testcontainers/testcontainers-go` 與 `modules/postgres` v0.44.0；`github.com/moby/go-archive` v0.3.3（testcontainers 的間接依賴，修 GO-2026-6253／CVE-2026-17106，2026-08-05 發布；Will 2026-10-18 於步驟 2a 核准）。間接依賴由 MVS 決定，`go mod tidy` 後若被拉高到冷卻期內的版本，回報。
  - Go 工具：`github.com/oapi-codegen/oapi-codegen/v2` v2.8.0、`github.com/sqlc-dev/sqlc` v1.31.1、`github.com/golangci/golangci-lint/v2` v2.14.0（Will 2026-10-18 核准冷卻期例外：v2.13.2 讀不了 go1.27.2 的 export data，實測 `export data version 5 is greater than maximum supported version 4`；v2.14.0 實測通過。從 release binary 安裝，checksum 在 `backend/Makefile`）、`golang.org/x/vuln` v1.8.0、`github.com/oasdiff/oasdiff` v1.33.0、`github.com/boumenot/gocover-cobertura` v1.5.0。
  - Atlas v1.3.3（D12，2026-10-18 由 v1.1.3 改版）：`https://release.ariga.io/atlas/atlas{,-community}-<os>-<arch>-v1.3.3`，2026-09-04 發布，sha256 取自同名 `.sha256`，寫在 `backend/Makefile`（standard）與 `backend/Dockerfile`（community）。Atlas 會把發布超過 6 個月的二進位從 CDN 移除（[CLI reference](https://atlasgo.io/cli-reference)「Distributed Binaries」）；v1.1.3 執行時已印出「will soon be deprecated and be unavailable for download」。`make atlas-age` 在發布滿 150 天時失敗，CI 的 backend job 會跑它。v1.3.3 實測（2026-10-18，隔離容器）：community 拒收多檔 `src`、standard 正常產生並在 `CONCURRENTLY` 檔頭加 `-- atlas:txmode none`、community 對全新 DB 重放兩支 migration 成功；對照組拿掉 directive 後 community 套用失敗（`CREATE INDEX CONCURRENTLY cannot run inside a transaction block`）。
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
  - 多實例：rate limit 改用 DB 或 proxy 層；零停機部署（docker-rollout 模式：Caddy 動態 upstream 加 `health_uri`、backend 不設 `ports`、shutdown 前加回 readiness drain）。觸發（任一）：backend 跑超過一個 replica、某個下游要求部署不能停機、改用 Kamal 或 Swarm（D14）。
  - managed Postgres（RDS 之類，`docker-entrypoint-initdb.d` 不會執行）：角色改由 runbook 以 superuser 建立，寫進 `docs/deploy-vps.md`。觸發：第一個用 managed DB 的下游（D15）。
  - `pg_hba` 只允許 superuser 從容器內的 socket 連線。觸發：D15 完成後的加固，或第一個正式部署（D15）。
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
- [x] **1. Walking skeleton**（被擋於：步驟 0）
  - 範圍：
    - 分支與保存：開 `go-rewrite` 分支；在開工時的 `main` 打 tag `archive/pre-go-rewrite` 並 push；開 draft PR。
    - 刪除 Python 後端：`backend/src`、`tests`、`alembic*`、`pyproject.toml`、`uv.lock`、`scripts/wait_for_db.py`、`.python-version`（若有）。
    - 建立 Go 骨架：
      - `go.mod`；`cmd/app`，子指令 `serve` 與 `healthcheck`（盤點 B22，設計審查補列）。
      - `internal/platform/config`（盤點 D 段、E10）、`internal/platform/logx`（slog）。
      - `internal/httpapi/server.go`：盤點 B19 的 timeout、body 上限、graceful shutdown（SIGTERM 後直接 `Shutdown`，上限 30s，D14）；`CrossOriginProtection`（B21）。
      - `GET /health`、`GET /health/ready`（E7）。
    - 建立 Atlas 設定：`atlas.hcl`、空的 `migrations/`。
    - 建置與部署：
      - `backend/Dockerfile`：multi-stage，內附 Atlas community CLI（P14 的 URL 與 checksum，`sha256sum -c` 驗證）。
      - `docker-compose*.yml`（F5）：migrate 與 backend 共用同一個 image（只 build 一次）；backend 的 `healthcheck`；PG 18 的 volume 掛在 `/var/lib/postgresql`（既有缺陷：掛在 `.../data` 時 `postgres:18` 拒絕啟動）；backend `stop_grace_period: 35s`（大於 30s 的 shutdown 上限，D14）。
      - `backend/Makefile`：`check` 先包含 gofmt、golangci-lint、`go test -race`、govulncheck。
      - root `Makefile` 的 `migration`、`generate-migration`、`dev`（F4 前半）。
    - 在 root `AGENTS.md`、`backend/AGENTS.md`、`docs/rules/*` 開頭加上改寫期間的提示行（見全域約束）。
      - `.env-example`（F6 的 backend 部分）。
    - CI 與 lint：
      - CI 的 Go job（F1 的 lint、test、govulncheck 部分）；安裝 Atlas standard 版並驗 checksum，供步驟 2 的漂移檢查使用；`make atlas-age`。
      - CI 的 stack job：build 正式 image，`docker compose up --wait backend`，斷言 migrate exit 0、`/health/ready` 回 200。image 內的 community 版因此每次都在 CI 跑過。
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
    - 送出 SIGTERM 後，進行中的請求做完才結束（`TestShutdownFinishesInFlightRequest`）。
      - mutation：`Shutdown` 換成 `Close`，測試要失敗。
    - `docker compose stop backend` 以 exit 0 結束（不被 SIGKILL）。
    - `TestBodyTooLarge` 回 413。
      - mutation：拿掉 `MaxBytesReader`，測試要失敗。
    - `TestSlowHeaderClosed`：測試把 `ReadHeaderTimeout` 設成 100ms，慢慢送 header，連線要被關閉。
      - mutation：拿掉 `ReadHeaderTimeout`，測試要失敗。
    - `TestCrossOriginPostRejected`：用一個暫時的 POST handler 測試，步驟 3 再換成真正的 handler。
      - mutation：拿掉 `CrossOriginProtection`，測試要失敗。
    - config 測試：照 `test-map.md` 的前 5 列。
    - draft PR 的 `gh pr checks` 全綠。
    - compose 的驗收在 scratchpad 的隔離 worktree 以 `.env-example` 產生的 `.env` 跑（project name `ft-verify`），不依賴 Will 的本機 `.env`；CI 的 stack job 也跑同一套。
  - 停止條件：
    - `atlas migrate apply` 對空目錄以非 0 結束 → migrate service 先直接 `exit 0`，步驟 3 補上，並記在進度表。
    - Atlas CLI 無法在 distroless 執行 → 回報；改用 Debian slim 之前先問。
  - 需要人做的事：Will 依 P12 更新本機 `.env`。
  - 完成後的狀態：前端登入暫時不能用。步驟 1–5 全部完成才合併。
- [ ] **2. 共用平台**（被擋於：步驟 1）
  - 步驟 2 拆成 2a–2e，各自有驗收，一個 session 不跨子步驟（2026-10-18 拆）。順序依相依：2a 的 harness 給 2b 的漂移檢查與之後的模組測試用；2d 的 429 與 request id 要用 2c 的 problem。步驟 2 完成 = 2a–2e 都完成。
  - 消費端（整個步驟 2）：無外部消費端，模組在步驟 3、4 才接上。各子步驟另列會受影響的既有檔案。
  - 不能動（整個步驟 2）：I2 的欄位名；步驟 1 的 timeout、body 上限、shutdown 行為與其測試。
  - 測試方式（2c、2d）：validator、rate limiter 與 problem 都用 `internal/httpapi/testdata/test-openapi.yaml` 加 stub handler 驗收，因為真正的 operation 要到步驟 3、4 才有。
  - [x] **2a. DB 角色、憑證、`app migrate` 與測試 harness**（D15；被擋於：步驟 1）
    - 範圍：
      - `deploy/postgres/initdb/10-roles.sh`（`docker-entrypoint-initdb.d`）：建 owner 角色（擁有 database 與 `public` schema，非 superuser、無 CREATEROLE）與 app 角色（只有 CONNECT）；`REVOKE ALL ON DATABASE … FROM PUBLIC`、`REVOKE CREATE ON SCHEMA public FROM PUBLIC`。角色名由 `DB_OWNER_USER`、`DB_APP_USER` 給（`.env-example` 預設 `fullstack_owner`、`fullstack_app`，`make rename` 在步驟 6 跟著改），密碼讀 compose secret 的固定路徑 `/run/secrets/db_owner_password`、`/run/secrets/db_app_password`（寫法同 FathomPod）。`POSTGRES_USER` 只用於初始化與容器內維運。
      - `internal/platform/migrate`＋`cmd/app migrate`：以 owner 身分執行 `atlas migrate apply --env deploy`。連線 URL 只經子行程的環境變數傳給 Atlas（`app migrate` 在暫存目錄產生只含 `deploy` env 的 Atlas 設定，讀 `MIGRATE_ATLAS_URL`；`backend/atlas.hcl` 移除 `deploy` env，image 不再帶它），子行程只拿到這個變數與 `HOME`、`TMPDIR`，不上命令列、不寫進 log；URL 帶 `lock_timeout=5s`（盤點 B7）。套用後以 owner 連線冪等授權：`GRANT USAGE ON SCHEMA public`、`GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES`、`GRANT USAGE,SELECT ON ALL SEQUENCES`、兩條對應的 `ALTER DEFAULT PRIVILEGES IN SCHEMA public`，全部在同一個 transaction。atlas 執行檔路徑與 migrations 目錄是參數（image 內是 `/usr/local/bin/atlas` 與 `/app/migrations`），harness 傳自己的值。
      - config：任何 `<KEY>` 都可改用 `<KEY>_FILE`（讀檔、去掉結尾的一個換行）；兩者同時設定是錯誤。`DATABASE_URL` 不含密碼，密碼另讀 `DATABASE_PASSWORD`／`DATABASE_PASSWORD_FILE` 後組進連線設定；URL 已帶密碼時啟動失敗。migrate 讀 `MIGRATE_DATABASE_URL` 與 `MIGRATE_DATABASE_PASSWORD_FILE`（owner）。serve 用 app 角色。
      - compose：top-level `secrets`（檔案在 repo 根的 `.secrets/`，git 忽略）；postgres 拿 `POSTGRES_PASSWORD_FILE` 與兩個角色的密碼檔；migrate 只拿 owner；backend 只拿 app；frontend 不拿任何 DB 變數。每個 service 用 `environment` 列出自己的變數（值從 `.env` 插值），不再 `env_file: .env`。
      - `make init-env`（`scripts/init_env.py`）：產生 `.env` 與 `.secrets/` 下三個密碼檔（目錄 `0700`、檔案 `0444`：檔案型 secret 是 bind mount，容器內的 postgres 與 nonroot 使用者要能讀；寫法同 FathomPod `init-state.sh`）；已存在就跳過，不覆蓋。`.gitignore` 加 `.secrets/`。`.env-example`、`backend/.env-example` 跟著改。
      - harness `internal/platform/dbtest`：testcontainers 起 PG 18，以同一支 `10-roles.sh` 初始化（`postgres.WithInitScripts`），呼叫 `internal/platform/migrate` 的同一段程式，以 owner 身分、community 版 Atlas 套用並授權，回傳 app 角色的連線。community 版由 `make tools` 下載到 `bin/atlas-community`，版本與 Dockerfile 相同、checksum 取自 release.ariga.io 的 `.sha256`；`make test` 依賴它。附一支帶 `-- atlas:txmode none` 的 testdata migration。
    - 會受影響的既有檔案：`rg -n "env_file|DATABASE_URL|POSTGRES_|migrate" docker-compose.yml docker-compose.dev.yml Makefile .github/workflows/ci.yml .env-example backend/.env-example backend/atlas.hcl backend/Dockerfile scripts/init_env.py`
    - 驗收：
      - `TestAppRoleCannotDDL`：app 角色執行 `CREATE TABLE`、`DROP TABLE` 都失敗，對 migrate 建立的表 `SELECT`／`INSERT` 成功。mutation：harness 改用 owner 連線，測試要失敗。
      - `TestMigrateGrantsIdempotent`：`migrate` 連跑兩次都成功；之後 owner 新建一張表，app 角色對它有 DML 權限。mutation：拿掉 `ALTER DEFAULT PRIVILEGES` 與 `GRANT … ON ALL TABLES`，測試要失敗。
      - `TestMigrateTxmodeNone`：testdata migration 含 `CREATE INDEX CONCURRENTLY`，套用成功。mutation：拿掉 directive，測試要失敗。
      - `TestMigrateLockTimeout`：另一條連線持有表的 `ACCESS EXCLUSIVE` 鎖時，套用一支改該表的 migration 在 10 秒內以錯誤結束。mutation：拿掉 `lock_timeout`，測試要失敗（以 context 逾時判定）。
      - `TestMigrateURLNotInArgs`：傳給 Atlas 的命令列參數不含密碼與 URL。mutation：改用 `--url`，測試要失敗。
      - config：同時設 `DATABASE_PASSWORD` 與 `DATABASE_PASSWORD_FILE` 時啟動失敗；`_FILE` 讀檔去掉結尾換行；`DATABASE_URL` 帶密碼時失敗。
      - compose：`docker compose config` 中 backend 看不到 owner 與 superuser 的密碼、frontend 看不到任何 DB 變數；ft-verify 隔離 worktree（`make init-env` 產生 `.env` 與密碼檔）：migrate exit 0、`/health/ready` 200、`docker compose exec postgres psql -U <app 角色> -c 'CREATE TABLE x()'` 失敗；CI stack job 改用 `make init-env` 後仍綠燈。
      - `make -C backend check` 綠燈、PR CI 綠燈。
    - 停止條件：
      - lib/pq 不把 URL 的 `lock_timeout` 當成 runtime parameter → 改用 `options=-c%20lock_timeout%3D5s`，兩者都不行就回報。
      - `postgres.WithInitScripts` 跑不了 `.sh`、或拿不到密碼檔 → 回報，不改成在 harness 裡重寫一份建角色的 SQL。
    - 需要人做的事：Will 在本機跑 `make init-env` 前先把舊 `.env` 移開（或手動補新變數與 `.secrets/`）；既有的本機 volume 已初始化過，角色不會自動建立，要 `make down-v` 重建（會刪本機資料）。
  - [x] **2b. 資料層工具與模組邊界**（I4、I5；被擋於：2a）
    - 範圍：
      - sqlc：`make sqlc` 與 `make sqlc-diff` 逐一處理 `internal/modules/*/sqlc.yaml`；沒有模組時通過。sqlc 以 `go run …@v1.31.1` 執行（版本經 Go checksum DB 驗證）；它要 cgo 編 libpg_query，本機首次編譯 8 分 53 秒，之後走 build cache。CI 在步驟 3 有模組後才會真的跑到 sqlc，屆時若每次都重編，改成 release binary 加 checksum（同 golangci-lint）。
      - `scripts/check-schema-drift.sh`：standard 版 `atlas migrate diff --env dev` 不得產生新檔；`scripts/check-migrations-immutable.sh`：對 `git merge-base HEAD origin/main` 已有的 migration 不得修改或刪除（寫法照 FathomPod 同名腳本）。drift 與 FathomPod 不同處：FathomPod 用 `atlas schema diff` 並在腳本裡重寫一次載入順序（`env://src` 不接受多檔，且 `--from` 為空目錄時直接報錯）；這裡對 `migrations/` 的暫存複本跑 `migrate diff --dir`（實測 `--dir` 會覆寫 env 的設定），沿用 `atlas.hcl` 的 `src` 順序、不寫進 repo，也順便擋下 `atlas.sum` 不符。
      - squawk 檢查 `migrations/*.sql`（`npx --yes squawk-cli@2.65.0`，`scripts/squawk.sh`），設定檔放 `backend/.squawk.toml`；`concurrent_index` policy 維持在 `atlas.hcl`。檔頭是 `-- atlas:txmode none` 的檔案加 `--no-assume-in-transaction`：Atlas 會把 `CONCURRENTLY` 與同次的其他變更放在同一支檔，squawk 讀不懂這個 directive，否則會誤報 `ban-concurrent-index-creation-in-transaction`（code review 實測）。
      - 架構測試 `internal/archtest`：用 `go list -deps` 斷言 `internal/modules/<x>` 不 import 其他模組、不 import `net/http`、不 import `internal/httpapi`；取代 `.golangci.yml` 裡「新增模組時手動補 depguard 規則」的註解與做法。只檢查非測試程式的依賴：模組的測試可以 import 別的模組來建 fixture（items 測試經 auth 建 user，滿足 `items.owner_id → users.id`）；depguard 的 `modules-no-http` 仍檢查每個檔案（含測試）的直接 import。
      - CI 的 backend job：`fetch-depth: 0`、setup-node（squawk）；PR 以 `origin/<base_ref>`、push `main` 以 `github.event.before` 作為 `BASE`；`main` 的 run 不再 `cancel-in-progress`，否則被取消的 run 會讓下一次 push 把改過的 migration 當成 base。
      - `make check` 加入 sqlc-diff、drift、squawk、migrations-immutable。
    - 會受影響的既有檔案：`backend/Makefile`、`backend/.golangci.yml`、`.github/workflows/ci.yml`（backend job 要 `fetch-depth: 0` 才有 merge-base）。
    - 驗收（mutation 在 scratchpad 的隔離 worktree 做，做完丟棄）：
      - 暫時新增 `internal/modules/zz/schema.sql` 建一張表、不產生 migration → drift 檢查失敗。
      - 暫時 commit 一支 migration 並當作 base，再修改它 → immutable 檢查失敗。
      - 暫時 migration 含對既有表的 `CREATE INDEX`（無 `CONCURRENTLY`）→ squawk 失敗。
      - 暫時兩個模組 `aa`、`bb`，`bb` import `aa` → 架構測試失敗；`aa` import `net/http` → 架構測試與 depguard 都失敗。
      - `make -C backend check` 綠燈、PR CI 綠燈。
  - [ ] **2c. 契約、problem 與 validator**（I1、I2；被擋於：步驟 1）
    - 範圍：
      - `api/openapi.yaml` 骨架：`servers: [{url: /api}]`，共用 schema `Problem`、`ValidationProblem`，cursor 分頁參數 `limit`、`cursor`，共用 problem responses。
      - oapi-codegen 生成到 `internal/httpapi/gen`（`go tool` 或 `go run` 鎖 v2.8.0，設定檔 `internal/httpapi/oapi-codegen.yaml`）；`make gen` 與 `make gen-check`（重新生成後 diff 必須為空）加入 `make check`。
      - `internal/platform/apperr`：kind（not-found、conflict、unauthenticated、forbidden、validation、rate-limited）與穩定 code。
      - `internal/httpapi/problem.go`：apperr → problem，`type` URI 照盤點 B2；未處理的錯誤回 500 並記 log（不把錯誤內容放進 `detail`）；panic recover；`*http.MaxBytesError` → 413；`CrossOriginProtection.SetDenyHandler` → 403 problem。
      - validator middleware（`internal/httpapi/validate.go`）：只掛在 `/api/` 子樹，`Options.Skipper` 跳過 dev 文件路由；kin-openapi 錯誤 → `errors[{pointer, code, detail}]`，code 對照 `required`、`too_short`、`too_long`、`pattern`、`enum`、`format`、`out_of_range`、`invalid_type`、`null_not_allowed`、`additional_property`，其餘 fallback `invalid`；Content-Type 不符回 415、`type` 為 `/problems/unsupported-media-type`；找不到路由回 404 problem。
      - dev 文件（只在 `MODE=dev`）：`GET /api/openapi.yaml`（內嵌的 spec）與 `GET /api/docs`（Swagger UI，swagger-ui-dist 5.33.0 從 jsDelivr 載入，鎖版本並帶 SRI）。
    - 會受影響的既有檔案：`internal/httpapi/server.go`（`NewHandler` 的組裝）、`cmd/app/main.go`。
    - 驗收：
      - 違反 `minLength` 的 request 回 422，`errors[0]` 有 `pointer` 與 `code: too_short`。mutation：code 對照表的 `minLength` 改成 fallback `invalid`，測試要失敗。
      - 對照表每個 code 各一個 table-driven case；未列的 kin-openapi 錯誤回 `invalid`。
      - Content-Type 不符回 415，`type` 為 `/problems/unsupported-media-type`。
      - `/api` 底下未定義的路徑回 404 problem。
      - `TestPanicReturnsProblem`：handler panic 時回 500 problem，log 有 stack。mutation：拿掉 recover，測試要失敗。
      - 未處理錯誤回 500，回應 body 不含錯誤字串。
      - `TestBodyTooLarge` 改斷言 413 problem；`TestCrossOriginPostRejected` 改斷言 403 problem。mutation：拿掉 deny handler，測試要失敗。
      - `GET /health` 經過完整 middleware 鏈仍回 200；dev 模式 `GET /api/openapi.yaml` 回 200（不被 validator 擋下）；prod 模式回 404。
      - gen-check：在 scratchpad worktree 改 `openapi.yaml` 一個欄位、不重新生成 → `make gen-check` 失敗。
      - `make -C backend check` 綠燈、PR CI 綠燈。
  - [ ] **2d. middleware 與分頁**（B4、B6、B9、E3、E4、E5；被擋於：2c）
    - 範圍：
      - request id：只採用 `TRUSTED_PROXIES` 來源送的 `X-Request-ID`，且限 ≤64 字的 `[A-Za-z0-9-]`；其餘重新產生（UUIDv7）；放進 context 與回應標頭，problem 帶 `request_id` 擴充欄位。
      - 安全標頭（`/api` 回應）：`X-Content-Type-Options: nosniff`、`Cache-Control: no-store`、`Referrer-Policy: no-referrer`；不送 `X-XSS-Protection`、`X-Frame-Options`、`Permissions-Policy`。
      - access log：method、path（不含 query）、status、`duration_ms`、`client_ip`、`request_id`。
      - client IP 解析：連線來源在 `TRUSTED_PROXIES` 內時，取 XFF 由右往左第一個不受信任的位址；否則用連線來源。
      - per-IP rate limiter（`x/time/rate`，in-memory，每 IP 一個 limiter，閒置項定期清除）：提供可套在指定 operation 的 middleware，步驟 3 掛到 `POST /api/users`、`POST /api/sessions`；超限回 429 problem 加 `Retry-After`。
      - `internal/platform/httpx` 的 cursor 編碼與解碼（base64url 編碼最後一筆的 id）；壞掉的 cursor 回 422 validation problem（pointer `/cursor`，code `invalid`）。
    - 驗收：
      - 不合法的 request id 被換掉；不受信任來源送的 request id 不被採用。mutation：拿掉格式檢查，測試要失敗。
      - `TestSecurityHeaders`：有 `Cache-Control: no-store`，沒有 `X-XSS-Protection`。mutation：拿掉 `no-store`，測試要失敗。
      - per-IP：來源在 `TRUSTED_PROXIES` 內時兩個不同 XFF 各自計數；來源不受信任時忽略 XFF；超限回 429 帶 `Retry-After`。mutation：改成一律採用 XFF，測試要失敗。
      - access log 有 `duration_ms` 與 `client_ip`。
      - cursor：編碼後解碼得回同一個 id；竄改過的 cursor 回 422。mutation：解碼不檢查長度，測試要失敗。
      - `make -C backend check` 綠燈、PR CI 綠燈。
  - [ ] **2e. CI 報表**（被擋於：2c）
    - 範圍：
      - oasdiff v1.33.0：比對 `origin/main` 的 `backend/api/openapi.yaml`，有 breaking change 就失敗；`main` 上沒有這個檔時印出跳過的原因並通過。
      - coverage：`go test -coverprofile`，gocover-cobertura v1.5.0 轉 Cobertura，PR 上用 diff-cover 10.5.1 算 changed-line coverage；兩個數字寫進 job summary，不設門檻（`AGENTS.md` Verification）。
    - 驗收：
      - PR #12 的 backend job summary 有 total coverage 與 diff-cover 的結果。
      - oasdiff 步驟的 log 顯示「main 沒有 openapi.yaml，跳過」。在 scratchpad 以兩份本機 spec 跑 oasdiff：刪一個 required 欄位要回報 breaking。
      - PR CI 綠燈。
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
    - 用 `atlas migrate diff baseline` 產生 baseline migration。產生前把 `atlas.hcl` 的 dev URL 改成 schema scope（`docker://postgres/18/dev?search_path=public`），因為所有表都在 `public`；重新評估：某個模組需要自己的 PG schema 時（設計審查 2026-10-18）。產生後用 community 版對全新 DB 套用一次，確認 revision 表位置。
    - `cmd/app seed-admin`（E8）：從內嵌的 `openapi.yaml` 取 `CreateUserRequest` schema，用 kin-openapi 驗證輸入（I1）；帳號已存在時不做任何修改，以 exit 0 結束。
    - root `Makefile` 的 `seed`、`reset-db`（F4 後半）：`reset-db` 以 `docker compose up -d --wait backend` 等待 readiness（盤點 B22）。
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
    - 同源部署：`deploy/Caddyfile`，以及正式 compose 的 `caddy` service、固定 subnet、`TRUSTED_PROXIES`（P10、F5 的正式環境部分）。Caddyfile 的 `reverse_proxy backend` 設 `lb_try_duration 10s`，backend 重建期間把請求留住；不設 `health_uri`（單一 upstream，D14）。部署文件（F10）在步驟 6 改寫。
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
  - 產生 migration 與漂移檢查用 standard 版（EULA、免費、不需登入），image 內套用用 community 版；不選「全用 community 加自寫合併腳本」。依據 P13。
  - 版本：v1.3.3（Will 2026-10-18 選 A）。v1.1.3 會因 Atlas 的 6 個月下架政策消失；不選「community 從原始碼建」（v1.x 沒發布成 Go module）與「改用 goose」。升版提醒由 `make atlas-age` 守住（P14）。
  - ADR：`docs/adr/2026-10-18-go-backend.md`（D2 修訂）。
- **D13 冷卻期的安全修補例外**（已決 2026-10-18，Will 照建議）
  - 寫在全域約束「依賴」。依據：Dependabot 的 cooldown 不擋安全更新；Go 1.27.2 修 `net/http`、`crypto/tls`，Caddy 2.11.4 有 GHSA-8qx7-j2v2-x6q5（≥2.11.5 修掉）。

- **D14 shutdown、health 與 proxy**（已決 2026-10-18，Will 照建議；擋住：1、5）
  - SIGTERM 後直接 `Shutdown`，上限 30s（不小於 `WriteTimeout`）；`stop_grace_period` 35s；不做 readiness drain。Docker healthcheck 打 `/health/ready`（compose 裡只用於 `depends_on` 與 `up --wait`，語意是 readiness）。步驟 5 的 Caddy 用 `lb_try_duration` 而非 `health_uri`。
  - 依據：Kubernetes「Termination of Pods」的 drain 前提是 LB 依 endpoint 分流；Kamal 與 docker-rollout 的零停機都在 proxy 層；Caddy 的 `lb_try_duration` 對單一 upstream 也有效。原 B19 的 drain 沒有來源，是照搬多 replica 的模式。
  - 零停機與多 replica 的終點與觸發見「範圍」的延後項，和 ADR api-and-auth 的「超過一個 replica」合併。
  - ADR：`docs/adr/2026-10-18-api-and-auth-baseline.md`（D8）。
- **D15 DB 角色與憑證**（已決 2026-10-18，Will 照建議；擋住：2）
  - superuser 只用於初始化與容器內維運；`<app>_owner` 給 migrate；`<app>_app` 只有 DML。角色由 initdb 腳本建立，授權由 `app migrate` 每次冪等執行；密碼用 compose secrets 加 `*_FILE`；測試用同一套角色。細節見步驟 2。
  - 舊系統共用 superuser 不是技術前提：舊 compose 本來就有獨立的 migrate service，只是共用 postgres image 的預設帳號與一份 `.env`（`git show archive/pre-go-rewrite:docker-compose.yml`）。
  - 依據：OWASP Database Security Cheat Sheet（最小權限；「should not be the owner of the database」）；PostgreSQL `ALTER DEFAULT PRIVILEGES` 只影響之後由該角色建立的物件；postgres image 的 initdb 只在空 volume 執行、支援 `_FILE`；Docker Compose secrets 文件。
  - ADR：`docs/adr/2026-10-18-go-backend.md`（D5）。

拍板順序：全部已決。P4、P9、P11 是預設，Will 可推翻；推翻時依 phase-plan 步驟 5 重寫受影響的步驟。

## 進度

狀態值：未開始／進行中／程式完成待驗收／完成。

| 步驟 | 狀態 | 已跑的驗收 | 未跑的驗收與原因 | commit |
|---|---|---|---|---|
| 0 查證與依賴核准 | 完成 | P13 五項已查並寫回；清單 Will 2026-10-18 核准（P14）；D12、D13 已決 | 無 | `4ae2473` 與本 commit |
| 1 Walking skeleton | 完成 | `make -C backend check` 綠燈；7 個 mutation 都讓指定測試失敗（503→500、MaxBytesReader、ReadHeaderTimeout、CrossOriginProtection、healthcheck 狀態判斷、lock_timeout、Shutdown→Close）；compose 驗收（隔離 worktree）：health、ready、migrate exit 0、port `127.0.0.1:8000`、停 postgres 後 503、`up --wait` 後 backend healthy、`docker compose stop backend` exit 0；設計審查 11 項已分流；D14 已實作；draft PR #12 在 `29d72d9` 的 `gh pr checks` 6 項全綠 | 無 | `9f014ed`…`29d72d9` |
| 2a 角色、憑證、migrate、harness | 完成 | `make -C backend check` 綠燈（本機）；mutation 10 項都讓指定測試失敗（default privileges、`GRANT … ON ALL TABLES`、`lock_timeout`（20s 逾時才失敗，有設定時 5.3s 結束）、`--url`、harness 改用 superuser、拿掉 txmode directive 並重算 atlas.sum、NewDB 漏 `GRANT CONNECT`、`_FILE` 兩者並存檢查、結尾換行、URL 帶密碼）；compose 驗收（ft-verify 隔離 worktree，`make init-env`）：migrate exit 0、`/health/ready` 200、backend 以 `fullstack_app` 連線、app 角色 `CREATE TABLE` 被拒、migrate 重跑 exit 0、`docker compose config` 中 backend 只有 `db_app_password`、migrate 只有 `db_owner_password`、frontend 沒有 secret 與 DB 變數 ；code review（獨立 subagent）7 項：1–6 已修（`7872d30`：initdb 缺密碼檔或空檔改為失敗並實測 exit 1、密碼改經 psql 環境、URL 的 `password` query 參數也拒絕、CI 兩個可能空過的斷言、`backend/.env-example`、init-env 拒絕空的殘留檔），7（`docs/deploy-vps.md` 過時）留給步驟 6 的 F10；GitGuardian 抓到 `command_test.go` 的測試假密碼（incident 38348098），check run 的 Skip 只放行單次，Will 選擇改寫歷史：`filter-branch` 把該檔在每個 commit 換成最終版本，最終 tree 與改寫前相同（`git diff` 為空），force push；PR #12 在 `5341e11` 的 `gh pr checks` 6 項全綠（GitGuardian：No secrets detected） | 無 | `c886bef`、`7872d30` |
| 2b 資料層工具與模組邊界 | 完成 | `make -C backend check` 綠燈（本機）；mutation（scratchpad 隔離 worktree）：只加 `zz/schema.sql` → drift 失敗且不寫進 repo，產生 migration 後通過；改 base 的 migration（含重算 `atlas.sum`）與刪除 → immutable 失敗；改檔不重算 → drift 失敗（checksum mismatch）；手寫無 `CONCURRENTLY` 的 `CREATE INDEX` → squawk 失敗，txmode none 檔內同樣寫法也失敗；Atlas 產生的 `CREATE TABLE`、單獨與混合的 `CONCURRENTLY` 檔 → squawk 通過，混合檔拿掉分組 → 失敗；query 改了不重生 → sqlc-diff 失敗；`bb` import `aa` → archtest 失敗；`aa` import `net/http` → archtest 與 depguard 都失敗；`Check` 不判模組間 → fixture 測試失敗；有 migration 沒有 schema.sql → drift 失敗；code review（獨立 subagent）：squawk 誤報混合檔（實測）已修、`main` 取消 run 讓 immutable 失效已修（不再 cancel）、測試檔跨模組 import 寫明為允許、drift 兩個邊界已修；`sqlc.yml` 等其他檔名不支援（慣例只用 `sqlc.yaml`）、squawk 也掃舊 migration（版本鎖死，升版時再處理）駁回；PR #12 在 `6d5b80b` 的 `gh pr checks` 6 項全綠 | 無 | `d2e067a`、`a09d553`、`6d5b80b` |
| 2c 契約、problem、validator | 未開始 | | | |
| 2d middleware 與分頁 | 未開始 | | | |
| 2e CI 報表 | 未開始 | | | |
| 3 auth | 未開始 | | | |
| 4 items | 未開始 | | | |
| 5 前端切換 | 未開始 | | | |
| 6 外圍與文件 | 未開始 | | | |
| 7 對照驗證 | 未開始 | | | |
| 8 收尾 | 未開始 | | | |
