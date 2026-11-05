---
title: 對外 API 的契約改用 Protobuf＋Connect，照 hawkr 的做法
date: 2026-11-03
review-date: 2027-02-03
last-reviewed:
status: active
supersedes: 2026-10-18-api-and-auth-baseline.md D2、D3、D9，以及 2026-10-18-api-and-auth-baseline.md D6 的「長度與格式只寫在契約」，與不變量「openapi.yaml 是唯一來源」
tags: [template-go-nextjs, decision, api, protobuf, connect]
---

# 對外 API 的契約改用 Protobuf＋Connect，照 hawkr 的做法

## Context

[API 與認證基準](2026-10-18-api-and-auth-baseline.md) D2 選了 spec-first 的 `openapi.yaml`：後端是 oapi-codegen strict server 加 kin-openapi validator，前端是 openapi-typescript 加 openapi-fetch，錯誤是 RFC 9457（D3）。[Go 後端](2026-10-18-go-backend.md) 2026-10-19 的 Review Note 決定先保留這套，並記下重新評估的觸發條件：每個 request 會被解析兩次，`validate.go` 與 `bind.go` 合計 353 行的轉接碼。repo 擁有者 2026-11-03 決定改用 hawkr 的做法（hawkr 的 Protobuf＋Connect 對外契約 ADR），讓自己的專案共用同一套契約工具與慣例。本 ADR 就是那次重新評估。

**約束**：

- `external`：template 沒有部署，也沒有外部 client（Will，2026-11-03），所以契約可以一次換掉，不需要相容性窗口。
- `external`：瀏覽器只看到一個來源，`/api/*` 由 Vercel 轉給 Caddy（[前端改由 Vercel 部署](2026-11-03-frontend-on-vercel.md)）。
- `inherited`：只有一個 web client，介面有 en 與 zh-TW 兩種語系（next-intl）。這是 template 的產品範圍，本次不改。
- `inherited`：衍生專案用 `make rename` 改寫 Go 的 module path（ADR go-backend D4）。仍成立：衍生專案的 Go 工具鏈以它為準。
- `inherited`：認證先於輸入檢查（threat model 的 EoP 表，`TestUnauthenticatedBeforeValidation`）。仍成立：它保護的是 schema 不對未登入的人揭露，與契約格式無關。
- `external`：connect-go 先解碼請求，才跑 interceptor（`connect@v1.21.1/handler.go:70` 在 `:83` 之前）。
- hawkr 選 Connect 的主要理由（兩支 Flutter App 的 Dart 生成、串流、四個 client）在這裡**不成立**。成立的理由是跨專案一致：Will 在 hawkr 與 template 衍生出的專案之間切換時，只需要會一套。

## Options Considered

- **基準：schema-first，IDL 是唯一來源，server 介面與 client 都由它生成，CI 擋住漂移與破壞性變更。** 第一方 client 的主流做法有兩種：OpenAPI（[OpenAPI Initiative](https://spec.openapis.org/oas/v3.1.0)，搭配 oasdiff）與 Protobuf 加 RPC（[Connect](https://connectrpc.com/docs/introduction)、gRPC；[Google AIP](https://google.aip.dev/)）。兩者都符合基準。
- **A. 維持 OpenAPI spec-first（現況）**：web 的 runtime 小（openapi-fetch 約 2.8 KB gzip），HTTP 語意（201、Location、204、merge-patch）直接對應 REST 的慣例。代價是要長期維護雙重解析的轉接碼，lint、breaking、生成是三套各自升版的工具，而且跟 hawkr 不同。
- **B. Protobuf＋Connect，照 hawkr（選用）**：buf 一個 CLI 涵蓋 lint、breaking 與生成；server 用 connect-go，web 用 protoc-gen-es、connect-es、connect-query。代價：web 多一份約 33 KB gzip 的 runtime（hawkr 量測）；REST 的 HTTP 語意要改寫成 RPC 的慣例。
- **C. 留在 OpenAPI，換成 [ogen](https://github.com/ogen-go/ogen)**：只解析一次，REST 形狀與 RFC 9457 不動，改動最小。go-backend 的 Review Note 提過它，但沒有做過 spike。
- **D. Protobuf 加 REST 轉碼（[vanguard-go](https://github.com/connectrpc/vanguard-go)、AIP-127）**：保留 REST 的 URL，但每支 RPC 要多維護一份路徑對應，而 REST 這一面沒有消費者。

子題（都在 B 之下）：

- **D1 版本與 breaking**：a `v1alpha1` 加 `ignore_unstable_packages`，上線時改名 `v1`（hawkr，[AIP-185](https://google.aip.dev/185)）；b 一開始就用 `v1`，`buf breaking` 對 `origin/main` 的 merge-base 檢查。
- **D2 輸入限制寫在哪**：a 模組的 Go 檢查，以欄位回報，轉成 `google.rpc.BadRequest`（hawkr）；b [protovalidate](https://buf.build/docs/protovalidate/) 註記，interceptor 執行。
- **D3 使用者看到的錯誤文字**：a 前端依 Connect code、`ErrorInfo.reason`、`FieldViolation.reason` 翻譯；b 伺服器送 `LocalizedMessage`（hawkr）。
- **D5 proto package 的根**：a 固定用 `app`；b 用專案名，`make rename` 一起改。（遷移期間證明行為沒變的測試做法記在計畫裡，不是長期決定。）

## Decision

- **B**：契約在根目錄 `api/`，路徑維持 `/api/`（`http.StripPrefix`）。依呼叫者分成 `<Module>PublicService`、`<Module>AppService`、`admin.<module>` 的 `<Module>AdminService`，授權跟著 service，取代 `operationAccess`。唯讀方法標 `NO_SIDE_EFFECTS`，以 GET 呼叫，回應維持 `no-store`。分頁照 AIP-158，部分更新用 `update_mask`（AIP-134），錯誤用 Connect code 加 `google.rpc` 的標準 details，`request_id` 只在 header。
- 取代 ADR api-and-auth-baseline D9：每個 Create 與 Update 都回傳資源，而且有對應的 Get；由 descriptor 測試檢查。
- 認證與限流：App 與 Admin 方法的未登入呼叫，在 net/http 這一層、Connect 解碼請求之前就擋下；只靠 interceptor 會讓未登入的人先看到解碼錯誤。`CreateUser`、`CreateSession` 的 IP 限流也放在這一層、解碼之前（照 hawkr；Will，2026-11-03）。
- 錯誤：Go 端的 details 用 `google.golang.org/genproto/googleapis/rpc` 的 `errdetails`，buf 不為 Go 生成 googleapis（同一個 full name 在 Go 的 registry 只能註冊一次）。轉成 `internal` 的錯誤由 interceptor 記 error log；interceptor 之前就回的錯誤都是 4xx，只留在 access log。apperr 的 `Conflict` 拆成 `AlreadyExists` 與 `FailedPrecondition`，各自對應一個 Connect code。不認得的 JSON 欄位被丟掉（connect-go 的預設），不再回 422。
- GET：照 hawkr，讀取用 GET（Will，2026-11-03，看過「一律 POST」的論點後選擇）。GET 不經過 CSRF 檢查，而且認證時會更新 session 的 `last_used_at`（`sessions.sql:14-15`）；這跟現在 REST 的 `GET /api/users/me` 相同，不是新增的風險。所有回應仍是 `no-store`。
- **D1 = b**、**D2 = a**、**D3 = a**、**D5 = a**（Will，2026-11-03；D1 先選 a，看過獨立草稿的論點後改選 b）。D1 的 breaking 規則是 `WIRE_JSON` 加 `RPC_NO_DELETE`、`PACKAGE_SERVICE_NO_DELETE`、`PACKAGE_NO_DELETE`：`WIRE_JSON` 本身不擋刪除 RPC、service 或 package（buf 1.50.0 的 fixture 實測）。proto 檔不寫 `go_package`，由 `buf.gen.yaml` 的 managed mode 加上 module path 的前綴。D2 的具體做法：限制寫在 `auth`、`items` 的 service 裡，`seed-admin` 與 `recover-admin` 經由同一個 service 方法共用。
- 刪除帳號仍要求 10 分鐘內登入過（[帳號濫用與生命週期](2026-10-22-account-abuse-and-lifecycle.md) D3）。它原本的理由之一「RFC 9110 沒有定義 DELETE 的 body」對 RPC 不再成立；留下的理由是前端沿用登入的呼叫做重新驗證，不必另外設計帶密碼的請求。

## Rationale

- **B 而非 A、C**：單看技術，只有一個 web client 時 B 和 C 接近：兩者都能消掉雙重解析，C 改動最小，還保留 REST 與 RFC 9457。決定性的理由是 repo 擁有者的目標「跟 hawkr 用同一套」（`inherited`，自己定的目標，不是外部強制）：規範、生成器與錯誤分類在專案之間共用，一人加 coding agent 維護多個 repo 時，少一套工具就少一組故障模式。B 另外的好處是 lint、breaking、生成收斂到 buf 一套，而且轉換的方向不對稱：proto 之後可以轉出 OpenAPI 或用 vanguard 提供 REST，handler 不動，反過來是重寫。代價是約 30 KB 的 runtime（hawkr 量的，這裡沒量）、REST 的 HTTP 語意與 Swagger UI 消失，以及一次性的改寫（21 支 API、約 120 個測試）。hawkr 的 Dart 與串流理由在這裡不成立，不拿來支撐 B。不選 C：沒有 spike，而且會讓 template 和作者的其他專案分岔。不選 D：多一層沒有消費者的對應。
- **D1 = b，跟 hawkr 不同**：本 repo 的分支模型下 `main` 就是已發布的程式（`dev`→`main` 才算發版），上線前的自由修改本來就發生在 `dev`，沒進 `main` 的契約怎麼改都不會被擋。a 要在上線時改名，每一支 procedure 的網址都會變；衍生專案忘了改名，就會在沒有 breaking 檢查的狀態下上線，正好碰上前後端分開部署的相容需求。hawkr 用 a，是因為它的遷移跨很多模組，`main` 也不等於已發布。代價：已進 `main` 的契約要做破壞性修改時，要在 `buf.yaml` 加 ignore 並帶 `Quality-Gate:`。遷移本身的 REST 刪除不受影響：oasdiff 在遷移的第一步就移除，因為沒有任何 client 依賴那些 path。
- **D2 = a**：限制和商業規則放在一起，CLI 與 HTTP 自然共用，不用新增依賴（protovalidate 帶進 cel-go），也跟 hawkr 相同。代價是限制不寫在 proto 上，前端拿不到機器可讀的限制；前端本來就只檢查必填。
- **入口限流**：舊的「無效的請求不扣 token」只是 strict middleware 排在 validator 後面的結果（`ratelimit.go:90-92` 的註解），沒有 ADR 或 threat model 要求它。放在入口，限流和未登入檢查在同一層，不需要 handler 先跑一次輸入檢查。代價：打錯的註冊或登入也會用掉額度（每 IP 每分鐘 5 次）。
- **D3 = a，跟 hawkr 不同**：template 有兩種語系，翻譯在前端（next-intl）已經有編譯期的完整性檢查；b 要把翻譯搬到後端並支援 `Accept-Language`。之後若要加 `LocalizedMessage`，多送一個 detail 不會破壞既有的 client。
- **D5 = a**：proto 的 package 不能有連字號，b 要訂轉換規則並搬動 `api/` 的目錄（buf lint 的 `PACKAGE_DIRECTORY_MATCH`）。這跟 ADR go-backend D4「識別應該是自己的」不一致；ADR go-backend D4 的理由是 `GOPRIVATE`、`go install` 與工具輸出以 module path 為準，proto 的 package 沒有這類工具依賴。兩個選項都要在 rename 後重新生成 Go 的程式碼：protoc-gen-go 把含 module path 的 `go_package` 帶長度前綴存進 raw descriptor。

## Expected Outcome

- repo 沒有 OpenAPI、oapi-codegen、kin-openapi、oasdiff、openapi-fetch、openapi-typescript；`git grep` 只剩 ADR 的歷史內容。
- `make check` 跑 `buf lint`、Go 與 TS 生成物的 drift、breaking 的 selftest；CI 全綠。
- 未登入的呼叫者送壞 JSON 給 App 方法，收到 `unauthenticated`；超過額度後，送給 `CreateSession` 的壞 JSON 收到 `resource_exhausted`；兩者都有測試。
- 在 `main` 上刪掉一支 RPC、一個 service 或一個欄位，`make api-breaking` 失敗。
- 新增一個 `ErrorReason` 卻沒有翻譯時，前端的型別檢查失敗。

## Followup

<!-- >>> template-only -->
- 執行步驟與進度在 [`docs/plans/2026-11-03-connect-contract.md`](../plans/2026-11-03-connect-contract.md)。
<!-- <<< template-only -->
- 計畫的步驟 3＋4 推送後，在瀏覽器實際走一次註冊、登入、重新整理、登出、刪除帳號（CI 沒有 E2E）。

## Invariants

- App 與 Admin 方法的未登入呼叫，在解碼之前就收到 `unauthenticated`。
- 只有 `Get*`、`List*` 可以標 `NO_SIDE_EFFECTS`：GET 不經過 CSRF 檢查。
- 輸入限制只有一個來源，HTTP 與 CLI 共用。
- `internal/modules/**` 不 import 生成的 proto 與 connect 套件。

## Revocation Triggers

- 出現不自己做翻譯的 client（例如 App）→ 重評 D3，加上 `LocalizedMessage`。
- 前端需要即時檢查與後端相同的限制 → 重評 D2。
- hawkr 或作者的其他專案離開 Connect → B 的決定性理由消失，重評 C。
- 出現第三方或需要 REST 的消費者 → 由 proto 轉出 OpenAPI 或加 vanguard，IDL 不換。
- 出現可以公開快取的讀取 → 該方法回 `public, no-cache` 加 ETag（hawkr 規範§九）。

## Review Notes

## Related

- 本 ADR 即原始紀錄：與 coding agent 在 2026-11-03 的對話中拍板（Will 回覆「照建議」；D1 與 GET 另外選擇）。起草時另有兩份獨立草稿，它們的論點（ogen 選項、`v1` 與分支模型、GET 的 CSRF 豁免）已在上文回答；之後的 design-review（同日）改動了 breaking 規則、限流位置與 supersedes 範圍。
<!-- >>> template-only -->
- 執行計畫與盤點：`docs/plans/2026-11-03-connect-contract.md`、`docs/plans/2026-11-03-connect-contract/inventory.md`。
<!-- <<< template-only -->
- 取代：[API 與認證基準](2026-10-18-api-and-auth-baseline.md) D2、D3、D9，以及 ADR api-and-auth-baseline D6 的「長度與格式只寫在契約」；ADR api-and-auth-baseline D1、D4、D5、D7、D8，以及 ADR api-and-auth-baseline D6 的其他部分照舊。重新評估的觸發：[Go 後端](2026-10-18-go-backend.md) 2026-10-19 Review Note。
- 參考實作：hawkr repo 的 `backend/docs/adr/2026-09-26-external-contract-protobuf-connect.md`、`backend/docs/adr/2026-09-25-api-error-model.md`、`backend/docs/開發規範/API 設計規範（Connect）.md`（hawkr 是私人 repo，取回方式：`git -C <hawkr> show fe7641a1:<path>`；三份文件最後一次修改在 `1868d8de`）。
