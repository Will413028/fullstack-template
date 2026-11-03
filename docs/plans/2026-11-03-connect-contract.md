# 對外 API 從 OpenAPI 換成 Protobuf＋Connect

**Goal：** 對外 API 的唯一來源是根目錄 `api/` 的 Protobuf。後端用 connect-go 提供 `/api/<package>.<Service>/<Method>`，前端用 protoc-gen-es 生成的 client 加上 connect-query 呼叫它；buf 負責 lint、breaking 與生成的漂移檢查。repo 裡沒有 OpenAPI、oapi-codegen、kin-openapi、oasdiff、openapi-fetch、openapi-typescript。終態以步驟 6 的驗收為準。

**上層：** 暫代（主清單在 repo 外，見本機個人脈絡檔）。

**附件：** [`2026-11-03-connect-contract/`](2026-11-03-connect-contract/)：`inventory.md`（舊系統的盤點，123 列，每列有去向，另有保留與改寫機制的沿用表；快照 commit `a83acc9`）、`review-ledger.md`（計畫審查與 design-review 的累積 finding）、`check-plan.py`（本計畫的一次性檢查）、`tests-map.md`（步驟 2 建立：每個舊測試對應的新測試，或刪除的理由）。

## 全域約束與完成定義（暫代主計畫）

- 參考實作是 hawkr 的 ADR `backend/docs/adr/2026-09-26-external-contract-protobuf-connect.md`（D1–D3）與〈API 設計規範（Connect）〉（hawkr repo `backend/docs/開發規範/`）。本計畫照它做；跟它不同的地方都寫在「決定」或「前提」，並附理由。
- 依賴：Will 2026-11-03 核准以下依賴，可以用更新的版本：`connectrpc.com/connect`、`google.golang.org/protobuf`、`protoc-gen-go` 與 `protoc-gen-connect-go`（放 `backend/tools/go.mod`）、`@bufbuild/protobuf`、`@bufbuild/protoc-gen-es`、`@connectrpc/connect`、`@connectrpc/connect-web`、`@connectrpc/connect-query`，以及 buf CLI（release binary 加 checksum）；2026-11-03 design-review 之後另核准 `google.golang.org/genproto/googleapis/rpc`（Go 端的 `errdetails`；hawkr 也用它）。不在這份清單裡的依賴（例如 protovalidate）要先列出版本、取得核准。
- 每一步都要符合 repo 的 `AGENTS.md`：後端 `make -C backend test` 與 `make -C backend check`，前端 `pnpm test`、`pnpm lint`、`pnpm build`；quality lock 往不好的方向動時，commit 帶 `Quality-Gate:` trailer；commit 超過 1,000 行時拆開，或帶 `Large-Change:`。
- 改認證、session、角色或 operation 權限的 commit，同一個 commit 要更新 `docs/security/threat-model-auth.md`（root `AGENTS.md`）。
- 機器高負載時，本機不跑需要 postgres 測試容器的檢查（`make test`、`make drift`），交給 CI，回報時標成「CI 待驗」（本機個人脈絡檔）。
- 本計畫完成前不開 dev→main 的發版 PR：中間的 dev 同時有 REST 和 Connect。
- 完成定義（每個服務都要做到）：proto 通過 `buf lint`；後端 handler 有整合測試，經生成的 Connect client 呼叫；舊測試全部列在 `tests-map.md`；該服務的 REST 路徑、handler 與 openapi 片段在同一步刪除；需要的文件同一步更新。

## 前提（開工時核對過，2026-11-03）

- 會改到或取代的既有部署是否在線：**不在線**。template 本身不部署到 Vercel 或 Cloudflare（Will，2026-11-03），repo 也沒有部署的 workflow。所以不需要相容性窗口，回滾就是 revert。步驟 1–4 期間 `dev` 上的 REST 與 Connect 雙軌是遷移的鷹架，步驟 5 刪除，不會進 `main`。
- 前端只呼叫認證相關的 API（`login`、`register`、`logout`、`getMe`、`deleteAccount`）；`items` 沒有前端呼叫端（`rg -n "/items|listItems|createItem" frontend/src -g '!**/schema.d.ts'` 只命中 `callback.test.ts:7`，那是頁面路徑，不是 API；已確認）。
- connect-go 先解碼請求，才跑 interceptor（`connectrpc.com/connect@v1.21.1/handler.go:70` 的 `receiveUnaryRequest` 在 `:83` 的 interceptor 鏈之前；hawkr 規範§八也這樣寫）。所以「認證先於輸入檢查」不能只靠 interceptor 的順序：App 與 Admin 方法的未登入呼叫，在 net/http 這一層、Connect handler 之前就擋下，依 URL 的 procedure 判斷 service 類別（已確認；維持現在 401 先於 400、415、422 的行為，inventory B2）。
- 限流照 hawkr 放在入口：net/http 這一層、未登入檢查旁邊、Connect 解碼之前，依 procedure 套用到 `CreateUser`、`CreateSession`（Will，2026-11-03，design-review 之後）。現在「無效的請求不扣 token」只是 strict middleware 排在 validator 後面的結果（`ratelimit.go:90-92` 的註解），沒有 ADR 或 threat model 要求它；改成入口限流後，打錯的請求也會用掉額度（每 IP 每分鐘 5 次），這是刻意的行為改變。登入節流照舊在 `auth.Service.SignIn` 裡（`sessions.go:93`）。限制照 D2-A 寫在 `auth` service 裡，`seed-admin`、`recover-admin` 經由同一個 service 方法共用。
- 路徑前綴維持 `/api/`：Caddy、`frontend/vercel.json`、`next.config.ts` 的 rewrite、CSP `connect-src 'self'` 都綁在 `/api/*`（inventory R1–R4、F12–F13）。後端用 `http.StripPrefix("/api", …)` 掛 Connect（預設，可推翻）。hawkr 用 `/api/connect/`，是因為它的 REST 與 Connect 長期並存；這裡不並存。
- proto package 的根固定是 `app`，例如 `app.items.v1`；`make rename` 不改它（D5）。
- 依呼叫者的類別各設一個 service，授權跟著 service 走：`<Module>PublicService`（不需要登入）、`<Module>AppService`（登入的使用者）、`admin.<module>` 底下的 `<Module>AdminService`（admin）。這取代 `operationAccess` 表（hawkr 規範§二；預設，可推翻）。
- 唯讀方法標 `NO_SIDE_EFFECTS`，client 用 GET 呼叫；App 與 Admin 的回應維持 `Cache-Control: no-store`（hawkr 規範§九；目前沒有 Public 的讀取）。GET 不經過 CrossOriginProtection 的檢查，前提是 GET 沒有副作用；這條由步驟 1 的 descriptor 測試守住。GET 把請求訊息編進 `?message=`，Caddy 與 Vercel 的日誌會記完整的 URI，所以請求裡帶憑證的讀取不標 `NO_SIDE_EFFECTS`（hawkr 規範§三；目前沒有這種讀取）。
- 分頁照 AIP-158：`page_size`、`page_token`、`next_page_token`，空字串表示最後一頁。token 的內容沿用現在的 keyset cursor 編碼（hawkr 規範§六）。
- 部分更新用 `google.protobuf.FieldMask update_mask`（AIP-134，hawkr 規範§七）。不導入 hawkr 的 etag 樂觀鎖：template 現在沒有並發修改的規則，這是新行為，不在本計畫的範圍（見「範圍」的不做）。
- 不用 `google.api.field_behavior`（與 hawkr 規範§四不同）：`api/` 不依賴 googleapis（design-review #36、#49），所以 server 設定的欄位（`id`、`owner_id`、`*_time`）寫在 proto 的欄位註解；`Update` 的可寫欄位由 handler 交給 `rpc.UpdatePaths(mask, resource, writable...)`，它檢查每個可寫欄位名都在 descriptor 裡，mask 列到資源的其他欄位時忽略，不是資源欄位的路徑回 `invalid_argument`（預設，可推翻；2026-11-03 步驟 2）。改用 `field_behavior` 的觸發條件：出現需要 `IMMUTABLE` 或 `INPUT_ONLY` 的欄位，或 `api/` 為了別的原因引入 googleapis。
- 錯誤：Connect code 加上 `google.rpc` 的標準 details（`ErrorInfo`、`BadRequest`、`RetryInfo`），`ErrorInfo.reason` 是 `common.v1.ErrorReason` 的值名（hawkr 規範§八）。使用者看到的文字由誰產生見 D3。`request_id` 只放在 `X-Request-ID` header，不放進 body。
- session 解析（`authenticate`）、CrossOriginProtection、MaxBytes、存取日誌與 panic recovery 都留在 net/http 這一層，在 Connect handler 的外面；它們擋下的請求由 connect-go 的 `ErrorWriter` 寫成 Connect 的錯誤（hawkr 規範§八）。
- connect-go 用 v1（v1.21.1，預設，可推翻）：v2.0.0 在 2026-10-07 發布，改成傳輸層無關的 `connect.Server`、interceptor 在解碼之前執行、生成的 handler 不再包 `Request`／`Response`；v1 在 `v1` 分支繼續維護，兩版可以在同一個 module 並存。本計畫的前提（解碼先於 interceptor，所以未登入檢查與限流放在 net/http 層）與 hawkr 都是 v1。改用 v2 是另一次遷移，觸發條件：v1 停止維護，或需要 v2 才有的功能。核對：2026-11-03，`gh api repos/connectrpc/connect-go/releases`。
- 不需要 spike：hawkr 已經在同一套 stack（connect-go、connect-es、connect-query、TanStack Query）上跑過。下列幾個未驗證的點，各自列在第一個用到它的步驟的停止條件裡：protoc 生成的 Go 是否被排除在 coverage 之外、knip 對生成的 TS 的反應、connect-query 2.3 與 TanStack Query 5.105 的相容性、SSR 用 connect-web transport 轉送 cookie。

**驗收基線：** `a83acc9` 在 CI 全綠（run 41586205101：Backend、Frontend、Compose stack、Rename smoke test、Commit policy、gate）。quality lock：`tests.backend: 241`、`tests.frontend: 41`、`suppressions.go: 16`（`scripts/quality-locks.json`）。沒有已知的失敗。

## 本階段依賴的不變量

| 不變量 | 依賴它的機制 |
|---|---|
| 認證先於輸入檢查：未登入的呼叫者在 App 與 Admin 的方法上一律收到 `unauthenticated`，看不到解碼錯誤、415 或欄位錯誤 | `internal/httpapi` 在 Connect handler 之前的 net/http 檢查（依 procedure 的 service 類別）；現在是 validator 的 `AuthenticationFunc` 與 `TestUnauthenticatedBeforeValidation`（`auth_test.go:712`）；threat model `docs/security/threat-model-auth.md:109` |
| 不洩漏他人資源是否存在：別人的 item、session、user 一律 `not_found` | `auth.Service.User`、`items` 的 owner 條件查詢；現在是 `TestGetOtherUsersItem`、`TestUpdateOtherUsersItem`（`items_test.go:243`、`:330`）、`TestGetOtherUserNotFound`（`auth_resources_test.go:111`）、`TestDeleteOtherUsersSession`（`:300`） |
| 每個 Create／Update 都回傳資源，而且有對應的 Get（取代 D9 的 Location 規則） | 步驟 1 的 descriptor 測試；`api-design.md` |
| 有副作用的方法不標 `NO_SIDE_EFFECTS`：GET 不經過 CSRF 檢查 | `NewHandler` 的啟動檢查：掛上去的 service 裡，標了 `NO_SIDE_EFFECTS` 的方法名必須是 `Get`、`List` 開頭；`server.go` 的 CrossOriginProtection |
| 送出的 `ErrorReason` 與欄位違規的 reason 都在 proto 裡宣告；前端每個值都有翻譯 | 步驟 1 的 Go 測試（取代 `problem_contract_test.go`）；步驟 4 的 `messages.test.ts` |
| 長度、格式等限制只有一個來源，HTTP、`seed-admin` 與 `recover-admin` 用同一份 | D2 選定的位置；`cmd/app/seed.go` 的 `validateAdminInput`（`:25`、`:46`、`:65`）；`docs/rules/database-design.md:66-67` 的 CHECK 對照 |
| session cookie 與 device cookie 的屬性不變（`__Host-`、HttpOnly、SameSite、Max-Age） | `internal/httpapi/auth.go` 的 cookie builder；`TestCreateUserSetsSessionCookie` 等（`tests-map.md`） |
| 限流與登入節流只套在 `CreateUser`、`CreateSession` | 入口的 IP 限流（依 procedure，在 net/http 層）；`auth.Service.SignIn` 的登入節流；threat model `:101` |
| 模組不依賴傳輸層：`internal/modules/**` 不 import 生成的 proto 與 connect 套件 | `.golangci.yml` depguard（`:60-72`）、`internal/archtest` |

## 規模（搬遷）

| | items | auth（users、sessions、sign-in lock）＋前端 | 產生計數的位置 |
|---|---|---|---|
| operation | 5（O17–O21） | 16（O1–O16） | `inventory.md` §1；`rg -c operationId backend/api/openapi.yaml` = 21 |
| httpapi 頂層測試 | `items_test.go` 16 | 其餘 107（14 個檔共 123） | `inventory.md` B32；`rg -c "^func Test" backend/internal/httpapi/*_test.go` |
| 前端測試 | 0 | 9 個檔（含 inventory F11 的 7 個與 `proxy.test.ts`、`callback.test.ts`） | `git ls-tree -r --name-only a83acc9 frontend/src \| rg '\.test\.tsx?$'` |

## 範圍

- 做：`inventory.md` 每一列照它的去向處理；proto、生成、CI 閘門；21 支 API 改成 Connect；前端改用生成的 client；文件、skills、ADR 改成 Connect 版。
- 不做：
  - etag 樂觀鎖（hawkr 規範§七）。這是新行為，不是搬遷；template 現在沒有並發修改的需求。
  - Swagger 的替代品（inventory B20）。Connect 沒有對應的 UI；proto 本身就是文件。
  - 串流、Bearer token、App client：都沒有呼叫端。
  - 公開讀取的 `ETag` 與 `public, no-cache`：目前沒有 Public 的讀取方法。
- 延後：
  - etag 樂觀鎖。觸發：第一個會被多人同時修改的資源。

## 步驟

- [x] **0. ADR 與計畫定稿**（被擋於：D1–D5 拍板）
  - 範圍：用 decision-log skill 寫 `docs/adr/2026-11-03-api-contract-protobuf-connect.md`：取代 `2026-10-18-api-and-auth-baseline.md` 的 D2、D3、D9 與「`openapi.yaml` 是唯一來源」這條不變量；在 `2026-10-18-go-backend.md` 的 Review Notes 記下這次重新評估的結果（inventory D7）。舊 ADR 只加 superseded 的指向，不改內容。把 D1–D5 的結論寫回本計畫的「決定」段。
  - 消費端：`rg -n "D2|D3|D9" docs/adr/2026-10-18-api-and-auth-baseline.md`；`rg -n "api-and-auth-baseline" --hidden -g '!node_modules' .`
  - 不能動：舊 ADR 的 D1、D4–D8 的內容（D6 裡「長度與格式只寫在契約」那一條除外，由新 ADR 取代）；`2026-10-22-account-abuse-and-lifecycle.md`、`2026-11-03-frontend-on-vercel.md` 的決定。
  - 驗收：`python3 docs/plans/2026-11-03-connect-contract/check-plan.py` 輸出 `ok`；新 ADR 的每一題都有選項、取捨與理由；design-review skill 審過計畫與 ADR，finding 照 Review protocol 處理。
  - 停止條件：D1–D5 有任何一題沒拍板。

- [x] **1. 工具鏈與共用層（walking skeleton）**（被擋於：步驟 0；D1、D2、D3、D5）
  - 範圍（檔案前的標記：新增、修改、刪除）：
    - 契約：（新增）`api/buf.yaml`（lint STANDARD；breaking `WIRE_JSON` 加 `RPC_NO_DELETE`、`PACKAGE_SERVICE_NO_DELETE` 與 `PACKAGE_NO_DELETE`（D1-B，沒有 `ignore_unstable_packages`；`WIRE_JSON` 本身不擋刪除 RPC、service 或 package：2026-11-03 以 buf 1.50.0 的 fixture 實測，少了 `PACKAGE_NO_DELETE` 時刪掉整個 package 是 exit 0，加上後刪除與改名 package 都 exit 100；同日在 1.75.0 以 `tools-selftest` 的情境再測一次，結果相同）；`ErrorReason` 的 `ENUM_VALUE_PREFIX` 例外，同 hawkr 的 `api/buf.yaml`）、`api/app/common/v1/errors.proto`（`ErrorReason`、`FieldViolationReason`）。`api/` 沒有 proto import googleapis，所以 `api/buf.yaml` 不宣告 deps、沒有 `buf.lock`（design-review #49）。
    - 測試用的契約：（新增）`backend/internal/httpapi/rpc/testdata/` 底下另一個 buf module（`buf.yaml`、`buf.gen.yaml`、`testing.proto`），Public、App、Admin 三類各一個 service，各有 `Get` 與 `Update` 方法。只生成 Go、只給 `internal/httpapi` 的測試掛載；不在 `api/`，所以不進前端的生成、不受 breaking 檢查，衍生專案也不會把它當成對外契約。`make gen` 與 `gen-check` 一併涵蓋它，跑 `buf lint`，留到步驟 6 都不刪。
    - buf CLI：（修改）root `Makefile` 照 hadolint 的做法加 `BUF_VERSION`、各平台的 `BUF_SHA256_*` 與版本戳記，以 `uname` 判斷平台、不需要 Go；`make bin/buf` 是唯一的下載點與 checksum 的唯一位置，backend 的 Makefile、前端的 CI job 與 lefthook 都呼叫它（Will，2026-11-03，design-review #51）。（修改）`scripts/tool_versions.py` 用現有的 `{file, version_var}` 方式追蹤 `BUF_VERSION`（T23）。
    - Go 依賴：（修改）`backend/tools/go.mod` 加 `protoc-gen-go`、`protoc-gen-connect-go`；（修改）`backend/go.mod` 加 `connectrpc.com/connect`、`google.golang.org/protobuf`、`google.golang.org/genproto/googleapis/rpc`。
    - 生成，生成物 commit 進 repo，Vercel 建置時不跑 buf：（新增）`backend/buf.gen.yaml` 生成到 `backend/internal/gen/`；Go 端的 details 用 genproto 的 `errdetails`，不為 Go 生成 googleapis（同一個 full name 在 Go 的 registry 只能註冊一次）；（新增）`frontend/buf.gen.yaml` 生成到 `frontend/src/generated/`；`google/rpc/error_details.proto` 以釘住 commit 的 `module: buf.build/googleapis/googleapis` 輸入另外生成（同 hawkr 的 `packages/hawkr-api-ts/buf.gen.yaml`）。
    - 過渡期的目標：（修改）`backend/Makefile` 把舊的 `gen`、`gen-check` 改名成 `gen-openapi`、`gen-openapi-check`，留在 `check` 裡到步驟 5；新的 `gen`、`gen-check` 跑 `buf generate`。前端同樣處理：舊的改名成 `gen:openapi`、`gen:openapi:check`，新的 `gen:api`、`gen:api:check` 跑 protoc-gen-es。（修改）`.github/workflows/ci.yml` 與 `lefthook.yml` 兩組都跑。
    - 閘門：（修改）`make api-breaking` 改跑（新增）`scripts/check-buf-breaking.sh`（保留 merge-base 與 `GIT_DIR` 的處理；base 沒有 `api/` 時略過，同現在的 oasdiff 腳本，T5）；（刪除）`backend/scripts/check-openapi-breaking.sh`、`backend/tools/go.mod` 的 oasdiff 與 selftest 的 oasdiff 情境，commit 帶 `Quality-Gate:`，理由是新 ADR 的約束（沒有部署、沒有外部 client）：步驟 2 起要從 `openapi.yaml` 刪 path，oasdiff 會擋住每一次 push。（修改）`make check` 加 `buf lint`；（修改）`backend/scripts/selftest-checks.sh` 加上「刻意違反 lint」「在 v1 fixture 刪欄位」「在 v1 fixture 刪一支 RPC」「在 v1 fixture 刪一整個 package」四個情境，都必須失敗（T6）；（修改）`scripts/check_commit_policy.py` 的檢查器設定清單加入 `api/buf.yaml`（T21）；（修改）`.gitattributes` 加入新的生成路徑（T11）；（修改）`.github/dependabot.yml` 把 `@bufbuild/*`、`@connectrpc/*` 設成同一組（T12）；（修改）`scripts/rename.py`：proto 的根固定是 `app`（D5），不改 proto；改寫 module path 時跳過 `backend/internal/gen/`，改完執行 `make -C backend gen`，讓生成物裡的 `go_package` 隨新的 module path 重新生成（protoc-gen-go 把 `go_package` 帶長度前綴存進 raw descriptor，文字替換會讓 init 時 panic）；（修改）CI 的 rename job 先 `make -C backend tools` 取得 buf，rename 之後跑 `make -C backend gen-check` 與 `go test ./internal/httpapi/rpc/...`（會載入 descriptor）（T24）；（修改）`.golangci.yml` 的 depguard 與 `internal/archtest`，禁止 `internal/modules/**` import `internal/gen/**` 與 connect（B33）。
    - 共用層：（新增）`internal/httpapi/rpc`，內容如下：
      - apperr 到 Connect code 與 details 的轉換（O23、B6），對應表在這一步定下：`Invalid`→`invalid_argument`、`NotFound`→`not_found`、`Unauthenticated`→`unauthenticated`、`Forbidden`→`permission_denied`、`RateLimited`→`resource_exhausted`（加 `RetryInfo`）。`Conflict` 拆成兩個 Kind：`AlreadyExists`→`already_exists`（`account_taken`）與 `FailedPrecondition`→`failed_precondition`（`last_admin`），依 gRPC 狀態碼的判準與 hawkr 的 error model ADR；其他一律 `internal`。
      - 轉換的 interceptor 把轉成 `internal` 的錯誤以 error 等級記 log（method、procedure、error，取代 `problem.go:126` 的 `unhandled error`）。connect-go 在 interceptor 之前就回的錯誤都是 4xx，`accessLog` 已經記下 procedure 與 status，不另外補記（design-review #50；舊系統也只對未知錯誤記 error）。
      - interceptor：recover → admin 檢查 → handler（B11）。
      - procedure 到 service 類別的表，由生成的 descriptor 產生；`NewHandler` 啟動時檢查每個掛上去的 service 名稱都以 `PublicService`、`AppService`、`AdminService` 結尾，不符就不啟動，這是唯一的類別檢查（B9、B10）。
      - AIP-158 分頁 helper，token 沿用 `internal/platform/httpx` 的 cursor 編碼；`httpx` 留到步驟 5，REST 不再用它時才刪（B24、B26）。`ParseID`（UUID）。
      - 入口的 IP 限流（沿用 `ratelimit.go` 的 token bucket 與 IPv6 /64 的 key），放在 net/http 層、未登入檢查之後、Connect 解碼之前，依 procedure 套用；步驟 3＋4 才把 `CreateUser`、`CreateSession` 加進清單（B12）。
    - 過渡期的 mux，（修改）`internal/httpapi/server.go`：
      - 外層保護照舊：request info、log、安全標頭、recover、CrossOriginProtection、MaxBytes。
      - `authenticate` 之後依路徑分流：`/api/app.` 開頭的進 Connect 這條路，其他照舊進 validator 與 REST 的 `BaseURL: "/api"`。
      - Connect 這條路依序做三件事：App 與 Admin 的未登入呼叫回 `unauthenticated`（依 procedure 的類別）、入口限流、`http.StripPrefix("/api", connectMux)`。
      - net/http 層只有一個寫錯誤的函式，用分流 mux 的同一個條件（路徑是 `/api/app.` 開頭）選格式：Connect 請求以 `connect.NewErrorWriter()` 寫 Connect 的錯誤，REST 照舊寫 problem。CSRF 的 deny handler、`authenticate` 的 DB 錯誤（`auth.go:91`）、外層的 panic recovery（`problem.go:149`）、未登入檢查與入口限流都用它；步驟 5 刪掉 REST 之後只剩 Connect 的分支（design-review #52）。不用 `ErrorWriter.IsSupported`：它把任何 `application/*` 的 POST 與任何 GET 都判成 Connect（`connect@v1.21.1/error_writer.go:79-88`）。MaxBytes 不需要選格式：connect-go 讀 body 時自己把 `*http.MaxBytesError` 轉成 `resource_exhausted`（`error.go:483-495`），它在 HTTP 上是 429，跟限流相同，前端靠有沒有 `RetryInfo` 分辨（B16、R4、B13）。
      - 路徑是 `/api/app.` 開頭、但 procedure 不存在（類別表裡查不到）：回 `not_found`，不先檢查登入（取代 `TestUnknownPathNotFound` 的 404 problem，B15）。
      - Connect 的錯誤 body 不放 `request_id`，只有 `X-Request-ID` header（B17）。
    - descriptor 檢查（新增），範圍取自 `NewHandler` 實際掛上去的 service，不靠手寫的套件清單（design-review #53）：
      - 每個 Create 與 Update 都回傳資源，而且有 Get。
      - `NO_SIDE_EFFECTS` 只標在 `Get*`、`List*`；這條併進啟動檢查，不符就不啟動。
      - 掃到的 service 數量大於 0（B30）。
      - 啟動檢查的測試：掛一個名稱不屬於三類的 service，`NewHandler` 要回錯誤（B10）。
    - 前端：（新增）`src/lib/api/` 的 Connect transport：baseUrl `<origin>/api`、`useHttpGet: true`、`defaultTimeoutMs`；錯誤解碼（`ConnectError` 加 `ErrorInfo`、`BadRequest`）；`unauthenticated` 導向登入的 interceptor：`ErrorInfo.reason` 是 `INVALID_CREDENTIALS` 時不導向（登入失敗留在頁面上），其他 `unauthenticated` 都導向；不靠 procedure 名單（預設，可推翻）。這時還沒有呼叫端，以單元測試驗收。舊的 `client.ts` 與 `errors.ts` 留到步驟 4。
    - 文件：（修改）threat model 加上過渡期的 Connect 入口與它的控制項（D10，同一個 commit）；（修改）`scripts/quality-locks.json` 的測試數，帶 `Quality-Gate:` 時理由指向 `tests-map.md`（T20）。
  - 消費端：`rg -n "operationAccess|limitOperations|requireRole|requireSession|validateRequests" backend/internal`；`rg -n "api-breaking|gen-check|gen:api|gen-openapi" backend/Makefile lefthook.yml .github/workflows frontend/package.json`；`rg -n "httpapi/gen|internal/gen" backend`
  - 不能動：現有 21 支 REST 的行為與測試（這一步只新增，REST 照常運作）；cookie 屬性；限流的參數。
  - 驗收：
    - `make -C backend check`、`pnpm lint && pnpm test && pnpm build` 與 `buf lint` 都通過；CI 的 Frontend job 用 `make bin/buf` 取得 buf，跑 `gen:api:check` 也通過。
    - 測試用的 service 經過完整的 `NewHandler` 呼叫得到；同一個 server 上 REST 的 CSRF 拒絕仍然回 problem，Connect 的 CSRF 拒絕回 Connect 錯誤（code `permission_denied`，reason `CROSS_ORIGIN_REQUEST`）。
    - mutation：
      - 讓 `check-buf-breaking.sh` 漏抓（例如直接 `exit 0`），`make tools-selftest` 要失敗；拿掉 `buf.yaml` 的 lint 規則，selftest 的 lint 情境要失敗；從 `buf.yaml` 拿掉 `RPC_NO_DELETE`，「刪一支 RPC」的情境要失敗；拿掉 `PACKAGE_NO_DELETE`，「刪一整個 package」的情境要失敗。
      - 轉換的 interceptor 不記 log，「`internal` 錯誤留下一筆 error log」的測試要失敗。
      - 測試裡對測試用的 Public service 套上入口限流；把限流移到 Connect 解碼之後，「超過額度後，送壞 JSON 的請求得到 `resource_exhausted` 而不是 `invalid_argument`」要失敗。
      - 把測試 proto 的 `Update` 標成 `NO_SIDE_EFFECTS`，descriptor 測試要失敗。
      - 測試裡讓 `NewHandler` 一個 service 都不掛，「數量大於 0」要失敗；新增一個沒列在任何清單裡的測試 service 並把它的 `Update` 標成 `NO_SIDE_EFFECTS`，啟動檢查要拒絕它。
      - 拿掉 admin interceptor，「user 呼叫 Admin 方法得到 `permission_denied`」要失敗。
      - 拿掉 net/http 層的未登入檢查，「未登入的呼叫者送壞 JSON 給 App 方法得到 `unauthenticated`」要失敗。
      - 共用的錯誤寫入函式改成一律寫 Connect 錯誤，「REST 的跨站 POST 仍然回 problem」要失敗；讓 `authenticate` 的 DB 錯誤繞過它，「Connect 路徑上 DB 錯誤回 Connect `internal`」要失敗。
      - 把「查不到的 procedure」改成先檢查登入，「`/api/app.x.v1.NopeAppService/Get` 未登入是 `not_found`」要失敗。
    - `make coverage` 的總覆蓋率沒有把 `internal/gen` 算進去（T22）；knip 沒有對 `src/generated` 報錯，或已在 `knip.config.ts` 加上附理由的 ignore（T17）。
    - `python3 docs/plans/2026-11-03-connect-contract/check-plan.py` 輸出 `ok`。
  - 停止條件：生成的 TS 在 `pnpm build` 失敗。遇到就停下來回報，不繞路。（connect-query 與 TanStack Query 的相容性移到步驟 3＋4，第一個呼叫端在那裡；審查 S11）
  - 完成後的狀態：Connect 的路由存在，只有測試用的 service；正式的 service 一個都沒有。

- [x] **2. items 搬到 Connect（小單元，先把做法跑通）**（被擋於：步驟 1；D2、D4）
  - 範圍：
    - （新增）`api/app/items/v1/items.proto` 的 `ItemAppService`：`CreateItem`、`GetItem`、`ListItems`、`UpdateItem`、`DeleteItem`。
    - （新增）handler；依 D2 的結論，把 title 1–200 等限制放到選定的位置。
    - （修改）`items_test.go` 的 16 個測試改用生成的 client；（新增）`tests-map.md`，每一個舊測試都列進去。
    - （刪除）items 的 REST 路徑、handler、openapi 片段與 `operationAccess` 的項目；（修改）重新生成 `api.gen.go`。
    - （修改）`scripts/quality-locks.json`。
  - 消費端：`rg -n "createItem|listItems|getItem|updateItem|deleteItem|/api/items" --hidden -g '!node_modules' -g '!*.gen.go' -g '!schema.d.ts' .`（前端沒有呼叫端，見前提）
  - 不能動：`internal/modules/items` 的 service 介面與查詢（`items.Patch` 已經能表達「有沒有指定」）；owner 條件；`defaultPageSize` 20、上限 100。
  - 驗收：
    - `make -C backend test`（高負載時交給 CI）與 `make -C backend check`。
    - `python3 docs/plans/2026-11-03-connect-contract/check-plan.py tests 'backend/internal/httpapi/items_test.go'` 輸出 `ok`；mutation：在 `tests-map.md` 刪掉一列，同一個指令要報出那個測試。
    - mutation：
      - `UpdateItem` 忽略 `update_mask`、改成整筆覆寫，「mask 只列 title，description 不變」要失敗。
      - 拿掉 owner 條件，「別人的 item 是 `not_found`」要失敗。
      - `ListItems` 不檢查 `page_token`，「無效的 token 是 `invalid_argument`」要失敗。
      - 拿掉 title 的長度上限，「201 個字的 title 是 `invalid_argument`，field 是 `title`」要失敗。
  - 停止條件：`update_mask` 無法在不改 `items` 模組介面的情況下表達「清空 description」。
  - 刻意的行為改變（寫進 `tests-map.md` 的對應列）：
    - 空字串的 page token 代表第一頁，不再是 422（AIP-158）。
    - `title: null` 的 `null_not_allowed`，改成「title 在 mask 裡但為空」得到 `invalid_argument`。
    - 不認得的 JSON 欄位被靜默丟掉，不再是 422 `additional_property`：connect-go 的 `protojson.UnmarshalOptions{DiscardUnknown: true}`（`connect@v1.21.1/codec.go:180`），Protobuf 為前向相容的預設。拼錯的欄位名（例如 `update_masks`、`updatemask`）不會報錯；`updateMask` 是 `update_mask` 正式的 JSON 名稱，照常接受。

- [x] **3＋4. auth 搬到 Connect，前端改用生成的 client**（被擋於：步驟 2；D2、D3；兩部分在同一次 push，可以分成多個 commit，但中間不 push：步驟 3 刪掉 auth 的 openapi 片段之後，前端在 `tsc` 就編不過，pre-push 會擋下）
  - 範圍，後端（步驟 3）：
    - （新增）`api/app/users/v1/users.proto`：`UserPublicService.CreateUser`；`UserAppService` 的 `GetCurrentUser`、`DeleteCurrentUser`、`GetUser`。
    - （新增）`api/app/sessions/v1/sessions.proto`：`SessionPublicService` 的 `CreateSession`、`DeleteCurrentSession`；`SessionAppService` 的 `ListSessions`、`GetSession`、`GetCurrentSession`、`DeleteSession`、`DeleteAllSessions`。
    - （新增）`api/app/admin/users/v1/users.proto`：`UserAdminService` 的 `ListUsers`、`GetUser`、`UpdateUser`、`DeleteUser`、`GetSignInLock`、`ResetSignInLock`。
    - （新增）handler 直接寫 `Set-Cookie`，兩個 cookie 用 `Header().Add`；（刪除）`withDeviceCookie` 的繞法（B21–B22）。
    - （修改）把 `CreateUser`、`CreateSession` 加進入口限流的 procedure 清單；登入節流在 service 裡，不在 handler。刻意的行為改變：無效的請求也會用掉額度，寫進 `tests-map.md` 與 threat model。
    - （修改）apperr 的 `Pointer`（JSON Pointer）改成模組回報欄位名、handler 對應到請求的欄位路徑；REST 在這一步刪除，所以不影響 REST 的輸出（B27）。
    - （修改）`cmd/app/seed.go`：`seed-admin` 與 `recover-admin` 改用與 `CreateUser` 相同的限制來源（B28）。
    - （修改）其餘 107 個 httpapi 測試與 `cmd/app` 的測試改用生成的 client，寫進 `tests-map.md`；（修改）CI 的 compose stack 探測改成 Connect 的路徑與預期的 code（R7），並加一個 Connect GET 帶 cookie 的 curl 探測：經 Caddy（帶 origin secret）POST `/api/app.users.v1.UserPublicService/CreateUser` 建一個帳號、存下 `Set-Cookie`；再 GET `/api/app.users.v1.UserAppService/GetCurrentUser?connect=v1&encoding=json&message=%7B%7D`，帶 cookie 預期 200 且 body 有那個 account，不帶 cookie 預期 401。
    - （刪除）對應的 REST 與 openapi 片段。
    - （修改）threat model 與 data inventory，跟它們描述的改動放在同一個 commit（D10、D11）。
  - 範圍，前端（步驟 4）：
    - （修改）`features/auth/api/auth-api.ts`；`server.ts` 改成 SSR 轉送 cookie、不快取，收到 `unauthenticated` 回 null；`hooks/use-auth.ts` 改用 connect-query；`auth-form.tsx`、`delete-account.tsx`（F4–F8）。
    - （修改）`messages/en.json`、`messages/zh-TW.json` 的錯誤段落改用 `errors.codes.<Connect code>`、`errors.reasons.<ErrorReason>`、`errors.fields.<FieldViolationReason>`（D3-A，F10）。
    - （修改）呼叫 API 的前端測試改成 Connect 的 wire；9 個前端測試檔都寫進 `tests-map.md`（F11）。
    - （刪除）舊的 `client.ts`、`errors.ts`、`schema.d.ts` 與 `gen:openapi*`。
    - （修改）`docs/frontend-known-issues.md` 的 timeout 說明（D19）。
    - （修改）兩邊的 `scripts/quality-locks.json`。
  - 消費端：`rg -n "/api/users|/api/sessions|sign-in-lock|users/me" --hidden -g '!node_modules' -g '!*.gen.go' -g '!schema.d.ts' .`（含前端、CI、文件）；`rg -n "validateAdminInput|api\.Spec|recoverAdmin" backend`；`rg -n "lib/api|authApi|ApiError|problemMessage|fieldErrors|fieldMessage" frontend/src`
  - 不能動：
    - `internal/modules/auth` 的語意：登入節流、裝置 cookie、最後一個 admin、重新驗證 10 分鐘、稽核紀錄。
    - cookie 屬性；`invalid_credentials` 對未知帳號與停用帳號回一樣的錯誤。
    - 前端的畫面、流程與文案（只改錯誤訊息的 key 與來源）；`proxy.ts` 的 cookie 名稱與 CSP。
  - 驗收：
    - `make -C backend test`（高負載時交給 CI）、`make -C backend check`、`pnpm lint && pnpm test && pnpm build`；CI 的 Compose stack job 通過，包含 Connect GET 帶 cookie 的探測。
    - `check-plan.py tests 'backend/*'` 與 `check-plan.py tests 'frontend/*'` 都輸出 `ok`。
    - `server.test.ts` 斷言 SSR 的請求是 GET 到 `/api/app.users.v1.UserAppService/GetCurrentUser`，而且帶著 request 的 `cookie` header。
    - mutation，後端：
      - 拿掉 device cookie 的 `Header().Add`，「第一次登入拿到兩個 cookie」要失敗。
      - `CreateSession` 不再結束請求帶來的舊 session，對應的測試要失敗。
      - `DeleteCurrentSession` 改成需要登入，「帶過期 cookie 也能登出」要失敗。
      - `DeleteSession` 對非目前的 session 也清 cookie，對應的測試要失敗。
      - 從入口限流的清單拿掉 `CreateSession`，「同一個 IP 第 6 次登入被擋下，帶 `RetryInfo`」的測試要失敗。
      - 拿掉 password 的 128 上限，對應的測試要失敗。
      - 讓 `seed-admin` 跳過共用的限制，對應的 `cmd/app` 測試要失敗。
    - mutation，前端：
      - 讓 `INVALID_CREDENTIALS` 也導向登入頁，「登入失敗留在登入頁並顯示錯誤」要失敗。
      - 在 proto 加一個 `ErrorReason` 並重新生成，沒有翻譯時 `pnpm lint` 要失敗。
  - 停止條件：`@connectrpc/connect-query` 2.3 與 `@tanstack/react-query` 5.105 不相容（安裝後 `pnpm build` 或型別檢查失敗；peer 範圍 `>=5.62.0` 已核對）；`CreateSession` 的節流需要的 client IP 或 device cookie，在 Connect handler 裡拿不到（應該在 context 裡，與現在相同）；connect-web 的 transport 在 Next.js 的 server 端無法帶上 request 的 cookie。
  - 需要人做的事：push 之後，在瀏覽器實際走一次註冊、登入、重新整理（SSR 讀得到 cookie）、登出、刪除帳號。CI 沒有 E2E，這是唯一的真實瀏覽器驗證；結果記在進度表。

- [x] **5. 刪除 OpenAPI，文件改成 Connect 版**（被擋於：步驟 3＋4）
  - 範圍：
    - （刪除）`backend/api/openapi.yaml`、`backend/api/embed.go`、`internal/httpapi/gen`、`oapi-codegen.yaml`、`validate.go`、`bind.go`、`problem.go` 的 problem 寫入、`spec_test.go`、`problem_contract_test.go`、dev docs（`docs.go`、`Deps.Docs`）、`gen-api.mjs`、`gen-openapi*` 目標，以及過渡期 mux 的 REST 分支。
    - （修改）`backend/Dockerfile`：刪掉 `COPY api api`，`make lint-docker` 要過。
    - （修改）`backend/go.mod`、`backend/tools/go.mod`、`frontend/package.json` 與它們的 lockfile：移除 kin-openapi、nethttp-middleware、nullable、oapi-codegen、openapi-fetch、openapi-typescript（oasdiff 在步驟 1 已移除）。
    - （刪除）`internal/platform/httpx`（cursor 編碼已搬到 `rpc`）。
    - （修改）`frontend/osv-scanner.toml` 刪掉 minimatch 的例外（T13，帶 `Quality-Gate:`）；`.github/dependabot.yml` 刪掉 speakeasy 的 ignore；`scripts/quality-locks.json` 依 `tests-map.md` 更新（帶 `Quality-Gate:`）。
    - 文件，都是修改：
      - `docs/rules/api-design.md` 改寫成 Connect 規範，從 hawkr 規範的一到十二節取 template 用得到的部分（D1–D5），包含§三「請求帶憑證的讀取不標 `NO_SIDE_EFFECTS`」。
      - `database-design.md`（D12）、三份 `AGENTS.md`（D13–D15）、README（D16）。
      - `.claude/skills/add-crud.md` 與 `add-module.md`（D17）、`.env-example`（D18）、`docs/deploy-vps.md` 的 curl（R6）。
      - 舊 ADR 引用的 REST 路徑只加註新的 RPC 名稱（D8）；程式註解（D20）。
  - 消費端：`git grep -n -i -E "openapi|oapi|kin-openapi|oasdiff|openapi-fetch|openapi-typescript|schema\.d\.ts|merge-patch|problem\+json|/problems/|COPY api" -- ':!docs/plans' ':(glob,exclude)**/go.sum' ':(glob,exclude)**/pnpm-lock.yaml' ':(glob,exclude)**/*.lock'`
  - 不能動：ADR 的歷史內容，只加 superseded 的指向。
  - 驗收：
    - 上面的 `git grep` 只剩 `docs/adr/` 底下的命中，每一筆以 `檔名:行號` 列在進度表：舊 ADR 的歷史內容，以及新 ADR `2026-11-03-api-contract-protobuf-connect.md` 說明它取代了什麼的段落。
    - `make -C backend check`、`make lint-docker`、`pnpm lint && pnpm test && pnpm build` 通過，CI 全綠；`osv-scanner` 沒有新的例外。
    - `check-plan.py tests` 輸出 `ok`。
  - 停止條件：刪掉某個檔之後，有一列盤點的去向還沒完成。

- [x] **6. 收尾**：完成定義逐項核對；`inventory.md` 每一列的去向都已完成（逐列打勾或附 commit）；design-review skill 審整個 diff（`git diff a83acc9..HEAD`）；`check-buf-breaking.sh` 與 descriptor 檢查已在 CI；延後項（etag）附觸發條件搬回主清單；用 `/code-review` 審過最後的 diff；確認步驟 3＋4 的瀏覽器驗證有紀錄。之後才開 dev→main 的 PR。驗收：`python3 docs/plans/2026-11-03-connect-contract/check-plan.py tests` 輸出 `ok`；CI run 的連結；design-review 的 finding 清單與處置。

## 決定

- **D1 proto 的版本與 breaking 閘門**（狀態：已決 2026-11-03，Will 選 B：先照建議選 A，看過兩份獨立 ADR 草稿的論點後改選 B；擋住步驟：0、1；需要的事實：無）
  - A：照 hawkr：package 一律 `v1alpha1`，`buf breaking` 用 `WIRE_JSON` 加 `ignore_unstable_packages`，所以上線前不檢查；閘門本身由 `tools-selftest` 的 v1 fixture 證明有效。衍生專案上線時一次改名成 `v1`，breaking 才生效。
    - 取捨：template 和衍生專案在上線前都能直接改契約，跟 hawkr 一致。代價是 template 現在的 `make api-breaking`（oasdiff 對 `origin/main`）會變成形同不檢查。
    - 會失敗的情況：有衍生專案已經上線，卻沒改名成 `v1`。這要靠延後項的觸發條件提醒。
  - B：一開始就用 `v1`，`WIRE_JSON` 從現在生效（Will 的選擇）。
    - 取捨：保留 template 現在「契約不能破壞」的閘門。代價是 template 自己的每次重設計都要繞過 breaking，跟 hawkr 的慣例不同，衍生專案也要學兩套規則。
  - 可逆性：A 改成 B 只要改 package 名稱和 `buf.yaml` 兩行；在沒有外部 client 的前提下，成本很低。
  - 改判的論點（兩份獨立 ADR 草稿各自提出）：本 repo 的分支模型下 `main` 就是已發布的程式，上線前的自由修改本來就在 `dev` 上；A 要在上線時改名，會換掉每一支 procedure 的網址，衍生專案忘了改名就會在沒有 breaking 檢查的狀態下上線。
  - 結論：B（一開始就用 `v1`，`buf breaking` 以 `WIRE_JSON` 對 `origin/main` 的 merge-base 檢查，base 沒有 proto 時略過；已進 `main` 的契約要破壞時，在 `buf.yaml` 加 ignore 並帶 `Quality-Gate:`）；ADR：`docs/adr/2026-11-03-api-contract-protobuf-connect.md`

- **D2 長度、格式等限制寫在哪裡**（狀態：已決 2026-11-03，Will 照建議；擋住步驟：1、2、3；需要的事實：無）
  - 現況：限制只寫在 `openapi.yaml`，由 kin-openapi 執行；`seed-admin` 也讀同一份 spec（inventory O22、B28）。換掉 OpenAPI 之後，這些規則需要新的家，否則會消失。
  - A：照 hawkr：模組在 service 裡檢查，用 apperr 回報欄位，handler 對應到請求的欄位路徑，轉成 `BadRequest`（hawkr 規範§八）。`seed-admin` 呼叫同一個 service 檢查，自然共用。DB 的 CHECK 照舊（建議）。
    - 取捨：不用新增依賴；規則在 Go 裡，跟商業規則放在一起；proto 只描述形狀。代價是限制不會寫在 proto 上，前端拿不到機器可讀的限制（前端本來就只檢查必填，F9）。
    - 會失敗的情況：之後需要在前端用同一份限制做即時檢查，這時要另外匯出限制。
  - B：protovalidate：限制寫在 proto 的 `buf.validate` 註記上，server 用 interceptor 檢查，`seed-admin` 對 message 呼叫 `Validate()`。
    - 取捨：限制寫在契約上，前端可以生成；這是 Buf 官方推薦的做法。代價是要新增依賴（`buf.build/go/protovalidate` 與它帶進的 `cel-go`，需要另外核准）；跟 hawkr 不同；錯誤的欄位路徑和 reason 要另外對應到 `FieldViolationReason`。
  - 可逆性：A 改成 B 要在每個 proto 加註記並換掉 service 裡的檢查，大約是每個欄位一處；B 改成 A 是反過來，成本相當。
  - 結論：A（限制寫在 `auth`、`items` 的 service 裡，HTTP、`seed-admin`、`recover-admin` 經由同一個 service 方法共用；入口限流後不需要另外匯出檢查函式）；ADR：`docs/adr/2026-11-03-api-contract-protobuf-connect.md`

- **D3 使用者看到的錯誤文字由誰產生**（狀態：已決 2026-11-03，Will 照建議；擋住步驟：1、4；需要的事實：無）
  - 現況：前端依 problem 的 `type`、`code` 與欄位錯誤的 `code`，用 next-intl 翻成 en 與 zh-TW（F10）；後端不送使用者看的文字。
  - A：維持前端翻譯：前端依 Connect code、`ErrorInfo.reason` 與 `BadRequest` 每個違規的 `reason`（AIP-193 的 `FieldViolation.reason`，值是 `FieldViolationReason`）翻譯；後端不送 `LocalizedMessage`（建議）。
    - 取捨：保留 template 的兩種語系與現在的翻譯流程，「每個值都有翻譯」的編譯期檢查也能沿用。代價是跟 hawkr 不同（hawkr 由伺服器送 zh-TW 的 `LocalizedMessage`）。
    - 會失敗的情況：之後出現 App 這類自己不做翻譯的 client。
  - B：照 hawkr：伺服器依 `Accept-Language` 送 `LocalizedMessage`，前端直接顯示。
    - 取捨：所有 client 的文字一致。代價是要把翻譯搬到後端、後端要支援兩種語系，`messages/*.json` 的錯誤段落會被刪掉。
  - 可逆性：A 之後再加 `LocalizedMessage` 不會破壞既有的 client（多一個 detail）；B 改回 A 要把翻譯搬回前端。
  - 結論：A（前端依 Connect code、`ErrorInfo.reason`、`FieldViolation.reason` 翻譯，後端不送 `LocalizedMessage`）；ADR：`docs/adr/2026-11-03-api-contract-protobuf-connect.md`

- **D4 證明行為沒變的安全網**（狀態：已決 2026-11-03，Will 照建議；擋住步驟：2；需要的事實：無）
  - A：逐一翻譯舊測試：每個舊測試在 `tests-map.md` 對應到新測試，或寫明刪除的理由（例如測的是 kin-openapi 的錯誤對應），由 `check-plan.py tests` 檢查沒有漏；刻意的行為改變逐條寫在對應列（建議）。
    - 取捨：做法簡單，舊的 123 個 httpapi 測試直接改寫。代價是新舊測試不是同一份程式碼，翻譯錯了要靠 mutation 抓。
  - B：照 hawkr ADR 的 D4：情境寫一次，透過 driver 介面操作，REST driver 打舊 handler、Connect driver 打新 handler，兩邊都要通過。
    - 取捨：同一份情境證明行為一致。代價是要先把 123 個測試改寫成 driver 形式，REST driver 在步驟 5 又會刪掉；hawkr 這樣做是因為它的 REST 與 Connect 要長期並存，這裡不會。
  - 可逆性：只影響遷移期間的測試寫法，完成後兩者的終態相同。
  - 結論：A（逐一翻譯，`tests-map.md` 加 `check-plan.py tests`）；ADR：不需要（遷移期間的做法，記在本計畫）

- **D5 proto package 的根要不要跟著 `make rename` 改**（狀態：已決 2026-11-03，Will 照建議；擋住步驟：1；需要的事實：無）
  - 現況：`rename.py` 把專案名寫進 Go module path（ADR go-backend D4）。proto 的 package 會出現在 wire 上（URL 是 `/api/<package>.<Service>/<Method>`），也會序列化進生成物的 descriptor。
  - A：固定用中性的根 `app`，例如 `app.items.v1`，`make rename` 不改 proto（建議）。
    - 取捨：rename 不必訂連字號的轉換規則，也不必搬 `api/` 的目錄（buf lint 的 `PACKAGE_DIRECTORY_MATCH`）。代價是衍生專案的 wire 路徑上沒有自己的名字。這跟 ADR go-backend D4「識別應該是衍生專案自己的」不一致；D4 的理由是 `GOPRIVATE`、`go install` 與工具輸出都以 Go module path 為準，proto 的 package 沒有這類工具依賴它，所以這裡接受中性的根。
    - 兩個選項都一樣：rename 改寫 module path 之後要重新生成 Go 的程式碼，因為 protoc-gen-go 把 `go_package`（含 module path）帶長度前綴存進 raw descriptor，文字替換會讓 init 時 panic（`scripts/rename.py:31`、`:55-63`；從程式碼推論，沒有實際跑過）。所以 rename job 一定要有 buf。
    - 會失敗的情況：衍生專案之後要和另一個也用 `app.` 的服務共用同一個 proto registry 或 gateway。
  - B：用專案名，`rename.py` 一起改。
    - 取捨：wire 上看得出是哪個專案。代價：NAME 允許連字號，proto 的 package 不行，要訂轉換規則；buf lint 的 `PACKAGE_DIRECTORY_MATCH` 要求目錄跟著搬；TS 的生成物也要重新生成。
  - 可逆性：package 還沒進 `main` 時改名只是重新生成；進了 `main` 之後，改名會被 breaking 檢查擋下（D1-B），要加 ignore 並帶 `Quality-Gate:`，而且所有 URL 都會換掉。
  - 結論：A（固定用 `app`）；ADR：`docs/adr/2026-11-03-api-contract-protobuf-connect.md`

拍板順序：D1–D5 彼此獨立，2026-11-03 一次拍板（Will：「照建議」）。

## 進度

步驟 1 的執行紀錄（2026-11-03）：

- 已跑的 mutation，每條都讓對應的測試失敗、還原後通過：`check-buf-breaking.sh` 直接 `exit 0`（4 個 breaking 情境失敗）、`STANDARD` 改 `BASIC`（lint 情境）、拿掉 `RPC_NO_DELETE`、`PACKAGE_NO_DELETE`、`PACKAGE_SERVICE_NO_DELETE`（各自的刪除情境）；轉換的 interceptor 不記 log（`TestInternalErrorLogged`）；拿掉 admin interceptor（`TestAdminServiceNeedsAdminRole`）；關掉 NO_SIDE_EFFECTS 名稱檢查、類別檢查（`TestNewMuxRejects`）；拿掉入口的未登入檢查（`TestConnectUnauthenticatedBeforeDecoding`）；關掉入口限流（`TestConnectRateLimitBeforeDecoding`）；`writeError` 一律寫 Connect（9 個 REST 測試）；查不到的 procedure 先檢查登入（`TestConnectUnknownProcedureNotFound`）；`connectServices` 一個都不掛（`TestConnectServicesReturnTheirResources` 等）；測試 proto 的 `UpdatePublicThing` 標 `NO_SIDE_EFFECTS`（`NewHandler` 拒絕啟動，7 個測試失敗）；前端 interceptor 對 `INVALID_CREDENTIALS` 也導向、`useHttpGet: false`；`src/generated` 過期時 `gen:api:check` 失敗。
- 與範圍的差異：測試 proto 另加三個「檢查必須拒絕」的 fixture service（`TestingInternalService`、`TestingSideEffectAppService`、`TestingOrphanAppService`），啟動檢查與 descriptor 檢查的反例用它們，不靠手寫清單；`NewMux` 另外拒絕串流方法（interceptor 只包 unary，admin 檢查會被繞過）；`ENUM_VALUE_PREFIX` 的例外以檔案為單位，`FieldViolationReason` 跟 `ErrorReason` 同檔，所以一起免前綴（兩者的值都是 wire 上的字串）；CI 的 rename job 用 `make bin/buf`，不是整個 `make -C backend tools`（只需要 buf）；`gen-check` 與 `gen:api:check` 比對 index，rename job 先 `git add -A` 再檢查。
- 留給後面步驟：`@connectrpc/connect-query` 還沒裝，步驟 3＋4 第一個呼叫端出現時再裝（沒有呼叫端時 knip 會報 unused）；相容性以 peer 範圍確認：connect-query 2.3.1 要求 `@tanstack/react-query >=5.62.0`（`npm view @connectrpc/connect-query@2.3.1 peerDependencies`），repo 用 `^5.105.0`。`rpc.PageSize` 照 AIP-158 把超過上限的 `page_size` 降到上限、負數回 `invalid_argument`；REST 現在對 `limit > 100` 回 422，這個行為改變在步驟 2 寫進 `tests-map.md`。

步驟 2 的執行紀錄（2026-11-03）：

- mutation 在 CI 跑（本機不起 testcontainers）：四條放在不合併的分支 `mutation/step2-items`，以 draft PR #63 觸發 run 41645367146，之後關閉 PR。結果：整筆覆寫 → `TestUpdateItemMask`（覆寫把 title 清空，`:318`）與 `TestUpdatedAtChanges` 失敗；`GetItem` 拿掉 owner 條件 → `TestGetOtherUsersItem`；`ListItems` 不檢查 `page_token` → `TestListItemsPageParameters`（field 變成 `cursor`）；拿掉 title 上限 → `TestCreateItemTitleLimits`（DB 的 CHECK 變成 `internal`）。`tests-map.md` 的 mutation：16 列逐一刪除，`check-plan.py tests` 每次都報出那一列。`rpc.UpdatePaths`、`rpc.Under` 在本機以單元測試先失敗後通過。
- 與範圍的差異：新增 `rpc.UpdatePaths`（AIP-134 的 mask，取代 hawkr 的 `field_behavior`，見前提）與 `rpc.Under`（模組欄位對應到請求路徑）；`items.Service` 的介面沒變，只在 `Create`、`Update` 裡加 `checkTitle`。REST 仍在用的分頁 helper 與 `TestDefaultPageSizeMatchesContract` 搬到 `users.go`、`auth_resources_test.go`；`TestOperationAccessEnforced` 少了 5 個 items 的 subtest，測試數鎖 269 → 277。
- 留給後面步驟：審查 S20–S22（`AGENTS.md` 規則 1、6 與 `add-crud` skill、`api-design.md` 的範例、page token 解兩次）都在步驟 5 處理；遠端分支 `mutation/step2-items` 刪除要 Will 同意。

步驟 3＋4 的執行紀錄（2026-11-03）：

- 停止條件都沒有觸發：connect-query 2.3.1 與 TanStack Query 5.105（探針用生成的 `ItemAppService` 呼叫 `useQuery`、`useMutation`，`tsc` exit 0；把參數型別寫錯時 `tsc` 報錯，證明探針有被檢查）；client IP 與 device cookie 在 Connect handler 的 context 裡（`sessions.go` 的 `CreateSession`）；SSR 用 connect-web 的 transport 帶 request 的 `cookie`（實作者以 `next start` 接假的 backend 實測：backend 收到 GET 與 cookie，過期的 cookie 導向登入頁）。
- 本機已跑：`buf lint`；`make -C backend` 不需 DB 的檢查（fmt-check、gen-check、gen-openapi-check、buf-lint、lint、deadcode、vuln、sqlc-diff、squawk、migrations-immutable、api-breaking、tools-selftest）；`rpc`、`platform/httpx` 的測試；httpapi 不需 DB 的 62 個頂層測試：以 `go test -c -overlay` 把 `TestMain` 換成不起容器的版本，shuffle 下全部通過（含新的 `TestUnauthenticatedBeforeValidation`、`TestRolesMapped`、`TestConnectBodyTooLarge`）；前端 `pnpm lint`、`test`（50 個）、`build`、`check:deps`、`check:arch`、`gen:api:check`；`check-plan.py tests` 輸出 `ok`，刪掉後端與前端各一列時各自報出那一列。前端 mutation（實作者跑）：`INVALID_CREDENTIALS` 也導向登入頁、刪一個 reason 的翻譯（`pnpm lint` 失敗）、SSR 不帶 cookie，三條都讓對應的測試失敗。
- 與範圍的差異：
  - descriptor 檢查改成在同一個 proto package 掛上的 service 裡找 `Get<X>`：`CreateUser`、`CreateSession` 在 Public service，對應的 Get 在 App service（舊的 `followLocation` 也是用回應設的 cookie 去讀）。
  - 一次回報所有無效欄位：模組把多個 `apperr.Invalid` 以 `errors.Join` 回傳（`apperr.Fields`），`rpc.Convert` 每個欄位一筆 violation；保留 REST validator「列出每個錯誤」的行為。`apperr` 的 `Pointer` 改成欄位名（B27）。
  - `authenticate` 清除無效 cookie 改在回應開始時做，handler 自己設了 session cookie 就不清（`clearStaleCookie`）：connect-go 把 handler 的 header 附加在已寫入的 header 後面，否則重新登入會同時送出清除與新的 cookie。
  - `openapi.yaml` 已經沒有 operation，REST 外殼（validator、strict server、problem、bind）仍編得過，照計畫留到步驟 5；以 operation 為單位、表格已空的機制（`operationAccess`、`adminOperations`、`requireRole`、`limitOperations`、`operations()`）在這一步刪除。
  - 從步驟 5 提前：前端的 `gen-api.mjs`、`gen:openapi*`、`openapi-fetch`、`openapi-typescript`（沒有使用者，knip 會失敗），CI 與 lefthook 的 `gen:openapi:check`。
  - `User.disabled` 是 `optional bool`：沒有 mask 的 `UpdateUser` 用明確的 false 就能啟用帳號（AIP-134 的「populated」）。`UserAppService.GetUser` 只回自己，admin 讀別人走 `UserAdminService.GetUser`（inventory O5；刻意的行為改變寫在 `tests-map.md`）。
  - 新增 `TestConnectBodyTooLarge`：Connect 路徑的 body 上限原本沒有測試（步驟 1 起），它是 `resource_exhausted` 且沒有 `RetryInfo`。
- 記下不做：前端對 body 超過上限（沒有 `RetryInfo` 的 `resource_exhausted`）顯示的是限流的文字，D3-A 的鍵沒有區分它；現在沒有任何前端呼叫送得出 1 MiB 的 body。觸發條件：第一個能送大 body 的呼叫（例如上傳）。
- 規則違反（派工造成）：後端測試的實作者照我的派工跑 `go test ./internal/platform/...`，`dbtest` 套件的 `TestMain` 在本機起了 PostgreSQL 容器（約 107 秒，已確認沒有殘留）；派工把 `platform/...` 寫成不需 DB 是錯的。前端的實作者停 `next start` 時用了 `pkill -f next-server`，可能停到別的 session 的 Next server。
- CI：run 41673635223（`4a21706`）全綠：Backend（`make check`，含 DB 測試、`make drift`、測試數鎖 303）、Frontend（測試數鎖 50）、Compose stack（含 Connect GET 帶註冊的 cookie 讀回帳號、不帶 cookie 是 401）、Rename smoke test、Commit policy。
- 後端 mutation 在 CI 跑：7 條放在不合併的分支 `mutation/step3-auth`，以 draft PR #64 觸發，之後關閉。第一次停在 `deadcode`（S40），第二次被一個 panic 中止（S41），第三次 run 41673873713 跑完 303 個測試、15 個失敗：M1 不送 device cookie → `TestFirstSignInSetsTwoCookies`、`TestDeviceCookieIsPerAccount`、`TestSignOutEverywhereForgetsDevices`、`TestUntrustedClientsLockedTrustedDeviceNot`；M2 不結束請求帶來的 session → `TestCreateSessionIssuesNewToken`；M3 登出要求登入 → `TestDeleteCurrentSessionClearsCookie`、`TestProcedureAccessEnforced`；M4 非目前的 session 也清 cookie → `TestDeleteSession`；M5 `CreateSession` 不限流 → `TestRateLimitAuthOperations`；M6 拿掉密碼上限 → `TestCreateUserLimits`、`TestCreateSessionLimits`；M7 `seed-admin` 跳過限制 → `TestSeedAdminRejectsShortPassword`。前端「proto 加一個 ErrorReason 不翻譯」以 second-brain 的 `scripts/red-proof.py` 在副本裡跑：`pnpm lint` 報 `Property 'RED_PROOF_REASON' is missing`。
- 審查：`/code-review high` 與 diff 視角審查者各一輪，S27–S41 見 `review-ledger.md`（13 條，沒有 blocking）。
- 瀏覽器走查（2026-11-04，Will 交代代做）：本機 `next dev` 加原生 PostgreSQL 18（不起容器）、Chrome 實際操作：註冊（密碼過短顯示「太短。」，後端 400）、登入後重新整理（SSR 帶 cookie，仍在 overview）、登出、登出後開 overview 轉登入頁、密碼錯誤顯示「帳號或密碼錯誤。」、登入、刪除帳號（錯誤密碼被擋、正確密碼後 `DeleteCurrentUser` 200）、刪除後登入被拒；資料庫剩 0 個 user、0 個 session。走查發現 `next dev` 下登入與註冊頁 500（S42），修正 `084e16a`（測試）、`acbd2ee`；production build 不受影響。遠端分支 `mutation/step3-auth` 經 Will 同意刪除。
- 留給後面步驟：步驟 5 的 `api-design.md`、skills、`biome.json` 與 `.gitattributes` 對 `schema.d.ts` 的引用、`osv-scanner.toml` 的 minimatch 例外、S28、S34、S37–S39。

步驟 5 的執行紀錄（2026-11-04）：

- 刪除：`backend/api/`（`openapi.yaml`、`embed.go`）、`internal/httpapi/gen`、`oapi-codegen.yaml`、`validate.go`、`bind.go`、`docs.go`、problem 寫入（`problem.go` 改名 `errors.go`，只寫 Connect 錯誤）、`spec_test.go`、`problem_contract_test.go`、`testdata/test-openapi.yaml`、`Deps.Docs`、`gen-openapi*`、mux 的 REST 分支；`internal/platform/httpx` 拆成 `rpc` 的 page token（`PageToken`、`PageAfter`，格式不變，已發出的 token 仍有效）與 `internal/platform/keyset`，service 不再解 token（S22、S34）。`gen-api.mjs` 與前端的 openapi 依賴已在步驟 3＋4 提前刪除。
- 本步驟新做的選擇（盤點表 B15 寫「REST 404 problem until step 5」，沒寫之後的格式）：不是 procedure 的路徑一律回 Connect 的 `not_found`，而且在查 session 之前判斷（S51）；`/health/ready` 在資料庫掛掉時回 `{"status":"unavailable"}`（503），和 200 的 body 同形狀。
- 依賴：`backend/go.mod` 移除 kin-openapi、nethttp-middleware、nullable、oapi-codegen runtime；`backend/tools/go.mod` 移除 oapi-codegen，重新 tidy 後多列出 `kr/pretty`、`rogpeppe/go-internal` 兩個既有工具的間接依賴。`frontend/osv-scanner.toml` 隨 minimatch 例外刪除（T13），dependabot 的 speakeasy ignore 刪除；osv-scanner 對三份 lockfile 0 筆。
- 鎖：後端測試 303 → 259（在 `3e85b7f` 跑退役的測試數得 45 個含子測試，加 TestTrim），CI run 41712163399 實際計數相符；Go suppression 15 → 14。
- 驗收 `git grep` 的命中只剩 `docs/adr/` 的歷史內容，與一筆誤報：`docs/adr/2026-10-18-api-and-auth-baseline.md:23,53,55,56,79,90,94,104,110`、`docs/adr/2026-10-18-go-backend.md:90,94`（舊 ADR 的決定與 Review Notes）；`docs/adr/2026-11-03-api-contract-protobuf-connect.md:7,15,29,30,32,54,55,63,86`（新 ADR 說明它取代了什麼）；`frontend/vercel.json:2` 是 Vercel 設定檔的 `$schema` 網址（`openapi.vercel.sh`），不是 OpenAPI 的引用。
- mutation（second-brain `scripts/red-proof.py`，副本內）：page token 不驗長度 → TestPageAfter 失敗；`keyset.Trim` 取錯列 → TestTrim 失敗；MaxBytes 不轉 `resource_exhausted` → TestConvert 失敗；拿掉 limit 防護 → TestTrim 失敗。
- 審查：`/code-review high` 7 條（S48–S54，全修，`9ce7fd8`）；diff 視角審查者 2 條（進度表沒列 ADR 命中、`vercel.json` 誤報沒記錄），由這段紀錄處理。S28 維持 recorded。

狀態值：未開始／進行中／程式完成待驗收／完成。

| 步驟 | 狀態 | 已跑的驗收 | 未跑的驗收與原因 | commit |
|---|---|---|---|---|
| 0 ADR 與計畫定稿 | 完成 | `check-plan.py` 輸出 `ok`；計畫審查 2 輪（32 條）；ADR 獨立草稿 2 份；design-review 2 輪（24 條）；buf 1.50.0 的 package 刪除 fixture；見 `review-ledger.md` | 無 | 本 commit |
| 1 工具鏈與共用層 | 完成 | CI 全綠：run 41617139179（`a381fea`）與 run 41618602232（`85381de`，含審查修正）：Backend（`make check`，含 `make test` 的 DB 測試與 `make drift`、測試數鎖 269）、Frontend（含 `gen:api:check`，測試數鎖 50）、Compose stack、Rename smoke test、Commit policy。本機：`make -C backend` 不需 DB 的全部檢查；httpapi 不需 DB 的 Connect 測試；前端 lint、knip、depcruise、test、build；rename 煙霧測試；coverage 排除 `internal/gen`；mutation 見下。審查：`/code-review high` 與 diff 視角審查者各一輪，13 條見 `review-ledger.md` S1–S13 | 無 | `323317b`、`807247a`、`387014a`、`6a39a76`、`a381fea`、`1e0fcd7`、`85381de` |
| 2 items | 完成 | CI 全綠：run 41645470222（`25c0008`）：Backend（`make check`，含 DB 測試、`make drift`、測試數鎖 277）、Frontend、Compose stack、Rename smoke test、Commit policy；前一個 run 41644558611（`68ca2d1`）測試全過、只有鎖數不符（S26）。mutation：CI run 41645367146，四條各自讓對應測試失敗（見上）；`check-plan.py tests` 的逐列刪除。本機：`make -C backend` 不需 DB 的檢查（fmt、gen、buf lint、lint、deadcode、vuln、sqlc-diff、squawk、migrations-immutable、api-breaking、tools-selftest）；前端 gen:api:check、gen:openapi:check、lint、knip、test、build。審查：`/code-review high` 與 diff 視角審查者各一輪，S14–S26 見 `review-ledger.md` | 無 | `d2dfa2c`、`49ddb90`、`68ca2d1`、`25c0008` |
| 3＋4 auth 與前端 | 完成 | CI 全綠：run 41673635223（`4a21706`，含 DB 測試、Compose 的 Connect GET 帶 cookie 探測、測試數鎖 303／50）；後端 7 條 mutation：CI run 41673873713；前端 mutation 4 條；`check-plan.py tests` 輸出 `ok`；審查 S27–S41；瀏覽器走查（2026-11-04，見上）與 S42–S47 | 無 | `ee08849`、`3755a1e`、`00c12f1`、`dbc7816`、`efe8c28`、`9a75fb6`、`4a21706`、`0224f57`、`084e16a`、`acbd2ee`、本 commit |
| 5 刪除 OpenAPI 與文件 | 完成 | CI 全綠：run 41712163399（`fd4cd51`，含 DB 測試、`make drift`、Compose stack、測試數鎖 259／52）與 run 41715963943（`9ce7fd8`，含審查修正）；本機：`make -C backend` 不需 DB 的全部子目標、`make lint-docker`（pre-push）、httpapi 不需 DB 的測試（overlay）、前端 lint／knip／depcruise／test／build、osv-scanner、`check-plan.py tests` 輸出 `ok`；mutation 4 條；審查 S48–S54 與 diff 視角 2 條 | 無 | `fd4cd51`、`dc907a1`、`9ce7fd8`、本 commit |
| 6 收尾 | 完成 | CI 全綠：run 41723215204（`41f68f6`，含 DB 測試、`make drift`、Compose stack、Rename smoke test、測試數鎖 259／52）；inventory 123 列逐列核對（唯讀 subagent，`inventory.md` §8）；design-review 整個 diff（S55–S58）；`/code-review medium`（S59–S67）；新測試 `TestEveryAppErrCodeIsDeclared` 的 red-proof 9 條都失敗；本機 `make -C backend` 不需 DB 的全部子目標、`check_locks.py`、`check-plan.py tests` 輸出 `ok`；`check-buf-breaking.sh` 在 CI（`make check` 的 `api-breaking`），descriptor 檢查在 CI 的 rename smoke test 與 `TestConnectServicesReturnTheirResources`；etag 搬回主清單；步驟 3＋4 的瀏覽器走查有紀錄（見上） | design-review 第 2 輪沒有派：S55 的修正由 `/code-review` 審過（S59–S67） | `41f68f6`、本 commit |
