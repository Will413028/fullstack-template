# API Design Rules（Connect）

本專案的對外 API 是 Protobuf＋Connect：根目錄 `api/` 的 proto 是 API 形狀的唯一來源，Connect handler 在 `backend/internal/httpapi`（共用的部分在 `internal/httpapi/rpc`），業務邏輯與輸入限制在 `backend/internal/modules/<m>` 的 service。決策來源：[ADR api-contract-protobuf-connect](../adr/2026-11-03-api-contract-protobuf-connect.md)、[ADR api-and-auth-baseline](../adr/2026-10-18-api-and-auth-baseline.md)、[ADR go-backend](../adr/2026-10-18-go-backend.md)。條文改寫自 hawkr 的〈API 設計規範（Connect）〉，只留本範本用得到的部分：本範本沒有串流、檔案回應、gateway，也沒有 etag 樂觀鎖（延後到第一個會被多人同時修改的資源）。

範例：`api/app/items/v1/items.proto`（使用者擁有的 CRUD）、`api/app/admin/users/v1/users.proto`（後台）。

## 一、檔案、package 與版本

- 一個模組、一個使用者類別一個 package，一個檔案：
  - 前台（公開或登入者）：`api/app/<模組>/v1/<模組>.proto`，package `app.<模組>.v1`。
  - 後台（admin）：`api/app/admin/<模組>/v1/<模組>.proto`，package `app.admin.<模組>.v1`（ADR api-contract-protobuf-connect Decision）。
  - 跨模組共用：`app.common.v1`，目前只有錯誤原因（`api/app/common/v1/errors.proto`）。
- package 的根固定是 `app`，從範本產生的專案都一樣（ADR api-contract-protobuf-connect D5）。
- 版本一開始就是 `v1`，不用 `v1alpha1`；`main` 就是已發布的程式（ADR api-contract-protobuf-connect D1-B）。見第十節。
- lint 是 buf 的 STANDARD；唯一的例外是 `errors.proto` 不檢查 `ENUM_VALUE_PREFIX`，因為值名就是 wire 上的 reason 字串（`api/buf.yaml`）。
- proto 不寫 `go_package` 等語言選項，由 `backend/buf.gen.yaml` 的 managed mode 加上 module path 的前綴（ADR api-contract-protobuf-connect Decision）。
- proto 不 import googleapis（沒有 `field_behavior`）：欄位由誰設定、是否必填寫在欄位註解，例如 `items.proto` 的 `// Set by the server.`、`// 1 to 200 characters (code points). Required.`。

## 二、Service 與授權

- 一個使用者類別一個 service，授權跟著 service 的名稱走（`internal/httpapi/rpc` 的 `ClassOf`）：

  | service 名稱 | package | 誰能呼叫 | 不符時 |
  |---|---|---|---|
  | `<Module>PublicService` | `app.<模組>.v1` | 任何人 | — |
  | `<Module>AppService` | `app.<模組>.v1` | 登入的使用者 | 未登入回 `unauthenticated`（`SESSION_INVALID`） |
  | `<Module>AdminService` | `app.admin.<模組>.v1` | 登入的 admin | 未登入回 `unauthenticated`；不是 admin 回 `permission_denied`（`INSUFFICIENT_ROLE`） |

- 名稱不是這三種結尾的 service，`rpc.NewMux` 拒絕掛載，server 不會啟動（`TestNewHandlerRejectsUnclassifiedService`）。
- App 與 Admin 的未登入呼叫在 net/http 這一層、Connect 解碼請求之前就擋下（`connectEntry`；ADR api-contract-protobuf-connect Decision、Invariants；`TestConnectUnauthenticatedBeforeDecoding`）。admin 的角色檢查在 interceptor（`rpc` 的 `requireAdmin`）。
- service 以外更細的條件（只能看自己的資料、「本人或 admin」）在模組裡判斷，條件寫進 SQL。
- 越權存取（已登入、資源存在但不屬於呼叫者）回 `not_found`，和「不存在」完全相同，寫入的方法也一樣，不讓人以回應的差異探測資源是否存在（ADR api-and-auth-baseline D7；`TestGetOtherUsersItem`、`TestUpdateOtherUsersItem`）。
- **NEVER** 先查出資源再在 handler 比對 owner：條件必須在 SQL 裡（`WHERE id = @id AND owner_id = @owner_id`）。UPDATE 與 DELETE 用 `:one` 或 `:execrows` 回報影響的列，0 列回 `not_found`，不可靜默成功（`internal/modules/items/queries/items.sql`）。
- role 只有 `admin` 和 `user`（`auth.RoleAdmin`、`auth.RoleUser`），註冊預設 `user`，admin 只能用 `app seed-admin` 建立（ADR api-and-auth-baseline D7）。

## 三、方法

- 標準方法照 AIP：`Get`（131）、`List`（132）、`Create`（133）、`Update`（134）、`Delete`（135）。其他動作是自訂方法（AIP-136），動詞加名詞，例如 `UserAdminService` 清除登入節流的方法。
- 每支 RPC 有自己的 `<方法>Request`、`<方法>Response`（buf lint 的 `RPC_REQUEST_STANDARD_NAME`、`RPC_REQUEST_RESPONSE_UNIQUE`）。`Get`、`Create`、`Update` 回傳 `{ <資源> <資源> = 1; }`，不直接回資源；`Delete` 回空的 `Delete<X>Response`，不用 `google.protobuf.Empty`（`items.proto`）。
- 每個 `Create` 與 `Update` 都回傳資源，而且同一個 package 裡有對應的 `Get`；取代 REST 時代「201 的網址要能 GET」的規則（ADR api-contract-protobuf-connect 的 Decision，取代 ADR api-and-auth-baseline D9；`TestConnectServicesReturnTheirResources`）。
- 集合的讀取是 `List`，一律分頁（第六節）。
- 沒有副作用的讀取標 `option idempotency_level = NO_SIDE_EFFECTS;`，client 以 GET 呼叫（第八節）。讀取不得順便寫入（例如記錄瀏覽數）；要記錄就另外設計寫入的方法。認證時更新 session 的 `last_used_at` 不算，那是 net/http 層的事（ADR api-contract-protobuf-connect Decision 的 GET 段）。
- 只有 `Get*`、`List*` 可以標 `NO_SIDE_EFFECTS`：GET 不經過 CSRF 檢查。其他名稱標了，`rpc.NewMux` 拒絕掛載（ADR api-contract-protobuf-connect Invariants；`TestNewHandlerRejectsSideEffectsOnGet`）。
- 請求帶憑證的讀取不標 `NO_SIDE_EFFECTS`：憑證（邀請碼、一次性 token 之類）放在請求訊息、以 POST 呼叫。GET 會把請求編進 URL，憑證就留在瀏覽器歷史、access log 與 proxy 的紀錄裡（照 hawkr §三）。session cookie 不算請求帶的憑證：以 cookie 認證的 `Get*`、`List*` 照常標，回應一律 `no-store`（第八節）。範本目前沒有這種讀取。

## 四、資源與欄位

- 欄位名稱是 snake_case；JSON 由 proto3 的對應轉成 lowerCamel（`owner_id` → `ownerId`）。
- ID 是 `string id`，值是 UUID（DB 的 UUIDv7），client 當成不透明的字串。不用 AIP-122 的資源名稱（`items/{id}`）。handler 以 `rpc.ParseID(path, s)` 解析，不是 UUID 回 `invalid_argument`，欄位是請求的路徑（`id`、`item.id`）。
- 時間是 `google.protobuf.Timestamp`，名稱以 `_time` 結尾（AIP-142）：`create_time`、`update_time`；DB 欄位仍是 `created_at`、`updated_at`（`docs/rules/database-design.md`），轉換在 handler（`items.go` 的 `toItem`）。**NEVER** 用 Unix timestamp 的整數。
- 數字：可能超過 2^53 的值用 `int64`（JSON 是字串）；筆數這類有界的值用 `int32`（JSON 是數字），例如 `SignInLock.failures`。
- 布林不加 `is_` 前綴（AIP-140）：`disabled`、`locked`。
- `optional`：零值是合法的值、而且要與「沒有填」區分時用，例如 `optional string description`（沒設是沒有描述，空字串是一段描述）。
- enum 的零值是 `<ENUM>_UNSPECIFIED = 0`，值帶型別名前綴（`Role.ROLE_ADMIN`）；DB 存模組 Go 常數的值（`auth.RoleAdmin`），轉換在 handler（`users.go` 的 `roles`）。client 遇到 `UNSPECIFIED` 或不認得的值走「未知」的呈現。
- 後台的訊息與前台分開：後台 package 需要額外欄位時自己定義訊息，相同的型別才 import 前台的定義（`app/admin/users/v1/users.proto` import `app/users/v1/users.proto`）。
- 敏感欄位（`password_hash`、session token 與其雜湊）**NEVER** 出現在任何回應訊息。
- 呼叫者不能設定的欄位（`id`、`owner_id`、`create_time`、`update_time`）在 `Create` 被忽略，在 `update_mask` 裡列出也被忽略（`TestCreateItem`、`rpc.UpdatePaths`）。

### 輸入限制（ADR api-contract-protobuf-connect D2）

- 長度、格式、範圍只在模組的 service 檢查一次，HTTP 與 CLI（`seed-admin`、`recover-admin`）經由同一個 service 方法共用。proto 上不寫限制（不用 protovalidate），handler 與前端的 zod **NEVER** 再寫一次。
- service 以 `apperr.Invalid(field, reason, detail)` 回報，`field` 是模組自己的輸入名稱（`title`），`reason` 是小寫的 `FieldViolationReason` 值名（`too_long`）。有幾個欄位不合法就用 `errors.Join` 一起回（`internal/modules/auth/limits.go`）。
- handler 以 `rpc.Under("item", err)` 把模組的欄位接到請求的路徑（`item.title`）。
- 長度以 code point 計（`utf8.RuneCountInString`），與 DB 的 `char_length` CHECK 相同；CHECK 是第二道防線，數值必須一致（`items.go` 的 `maxTitle`、`items/schema.sql`）。

```go
// 正確：internal/modules/items/items.go，限制在 service
case n > maxTitle:
	return apperr.Invalid("title", "too_long", fmt.Sprintf("title must be at most %d characters", maxTitle))
```

```typescript
// 正確：前端只檢查必填，長度與格式交給後端的 field violation（frontend/src/features/auth/validations.ts）
// 錯誤：在 zod 重寫 service 的限制，兩處遲早不一致
account: z.string().min(3).max(50).regex(/^[A-Za-z0-9_.-]+$/),
```

## 五、分頁（AIP-158）

- 請求：`int32 page_size`、`string page_token`；回應：`string next_page_token`，空字串表示最後一頁。
- `page_size` 為 0 時用預設值，超過上限以上限計，負數回 `invalid_argument`（`page_size`、`OUT_OF_RANGE`）。預設值與上限寫在欄位註解，與 handler 的常數一致（`items.go` 的 `defaultItemsPage`、`maxItemsPage`；`TestListItemsPageParameters`）。
- `page_token` 是不透明的字串：上一頁最後一筆的 id（keyset，不是 offset）。不是這個 API 發的 token 回 `invalid_argument`（`page_token`、`INVALID`）。
- handler 以 `rpc.PageRequest(size, token, def, max)` 讀參數、`rpc.PageToken(next)` 產生回應的 token；模組以 `keyset.Rows(limit)` 多取一筆、`keyset.Trim` 切成一頁並得到下一頁的起點（`internal/platform/keyset`）。
- 排序以 UUIDv7 的 `id DESC`（等於建立時間新到舊）；不提供總數，不跑 `COUNT(*)`。需要頁碼的後台表格才另外設計，且只限該方法。

## 六、部分更新（AIP-134）

- `Update<X>Request` 是 `<X> <x> = 1; google.protobuf.FieldMask update_mask = 2;`，`<x>.id` 指定資源。
- 只改 mask 列出的欄位；`"*"` 是所有可修改的欄位；沒帶 mask 時視為列出所有有值的可修改欄位，所以要清空或改成零值的欄位必須列出來。列到的 `optional` 欄位沒有值就是清掉。
- 路徑只支援最上層的欄位；伺服器設定的欄位列出時忽略，其他不認得的路徑回 `invalid_argument`（`update_mask`）。
- handler 以 `rpc.UpdatePaths(mask, resource, writable...)` 取得要改的欄位，可修改的欄位清單是 handler 的變數（`items.go` 的 `itemWritable`），再組成模組的 `Patch`；模組區分「不改」與「清掉」，把「有沒有選到」明確傳給查詢（`items.Patch` 的 `SetDescription`、`items.sql` 的 `@set_description`）。
- 表單送出時列出表單上的欄位，不用 `"*"`：日後資源新增欄位時，舊版的 client 不會把它清掉（照 hawkr §七）。
- 有欄位被改的 `Update` 一律更新 `update_time`，即使值沒變（`items.proto`、`TestUpdatedAtChanges`）。

```go
// 正確：internal/httpapi/items.go
paths, err := rpc.UpdatePaths(req.Msg.GetUpdateMask(), in, itemWritable...)
if paths["description"] {
	p.SetDescription, p.Description = true, in.Description
}
```

## 七、錯誤

- Connect code 表示類別，依 gRPC 的判準選：要建立的東西已存在是 `already_exists`，系統不在這個操作需要的狀態、重試也沒用是 `failed_precondition`（ADR api-contract-protobuf-connect Decision 的錯誤段）。
- details 只用 `google.rpc` 的標準型別（Go 端是 genproto 的 `errdetails`），每種最多一個：
  - `ErrorInfo`：client 需要在同一個 code 底下分支時才帶；`reason` 是 `app.common.v1.ErrorReason` 的值名（不加前綴），`domain` 是 `app`（`rpc.ErrorDomain`）。
  - `BadRequest`：`invalid_argument` 能歸到欄位時，每個違規的 `field` 是請求訊息的 proto 路徑，`reason` 是 `FieldViolationReason` 的值名（AIP-193）。
  - `RetryInfo`：知道多久之後能重試時（登入節流）。
- 伺服器不送給使用者看的文字，不送 `LocalizedMessage`：前端依 Connect code、`ErrorInfo.reason`、`FieldViolation.reason` 以 next-intl 翻譯（ADR api-contract-protobuf-connect D3）。`message` 給開發者看，client **NEVER** 顯示或解析。
- request id 只在回應的 `X-Request-ID` header，不在 body（ADR api-contract-protobuf-connect Decision）。
- 模組錯誤到 Connect 的轉換只在 `internal/httpapi/rpc` 一處（`Convert`、`kindCodes`），由 interceptor 執行：

  | apperr kind | Connect code | details |
  |---|---|---|
  | `Validation`（`apperr.Invalid`） | `invalid_argument` | `BadRequest` |
  | `NotFound` | `not_found` | — |
  | `AlreadyExists` | `already_exists` | 有 code 時 `ErrorInfo`（`ACCOUNT_TAKEN`） |
  | `FailedPrecondition` | `failed_precondition` | 有 code 時 `ErrorInfo`（`LAST_ADMIN`） |
  | `Unauthenticated` | `unauthenticated` | 有 code 時 `ErrorInfo` |
  | `Forbidden` | `permission_denied` | 有 code 時 `ErrorInfo` |
  | `RateLimited` | `resource_exhausted` | `ErrorInfo`；有 `RetryAfter` 時加 `RetryInfo` |
  | 其他 error、panic | `internal` | 無；內容只進 error log |

- service 只回 `*apperr.Error`（`apperr.New(kind, code, detail)`、`apperr.Invalid(field, reason, detail)`）。`detail` 會送給 client：不得含 SQL、主機名稱、secret 或其他使用者的資料。modules 不 import `net/http`、`connectrpc.com/connect` 與生成的 proto（depguard、`internal/archtest`；ADR api-contract-protobuf-connect Invariants）。
- 新的 reason 一定先宣告在 `api/app/common/v1/errors.proto`：沒宣告的 `ErrorInfo` reason 不送出、欄位的 reason 退成 `INVALID`，兩者都記 log（`rpc` 的 `errorReason`、`fieldReason`、`LogConverted`；`TestEveryAppErrCodeIsDeclared` 從原始碼讀出每個 `apperr.New`、`apperr.Invalid` 呼叫與 `apperr.Error` literal 的 code；事後才指定的 code 看不到）。新增的順序：`errors.proto` → `make -C backend gen` 與 `pnpm gen:api` → 每個語系 `frontend/messages/*.json` 的 `errors.reasons` 或 `errors.fields`（`messages.test.ts` 在編譯期檢查涵蓋每個值）→ 模組回報。
- 宣告過的值可以新增，**NEVER** 改名、刪除或換 code（`errors.proto` 的檔頭）。

```go
// 正確：service 回報預期中的失敗
return apperr.New(apperr.AlreadyExists, CodeAccountTaken, "account is taken")

// 錯誤：service 決定傳輸層的 code（modules 不能 import connect，depguard 會擋）
return connect.NewError(connect.CodeAlreadyExists, err)
```

## 八、讀取、快取、大小與 wire 格式

- API 與前端同源，掛在 `/api` 底下：procedure 的網址是 `/api/<package>.<Service>/<Method>`，例如 `/api/app.items.v1.ItemAppService/ListItems`；handler 以 `http.StripPrefix("/api", …)` 掛載（`server.go`；ADR api-contract-protobuf-connect Decision）。Vercel 把 `/api/*` 轉給 API 的 Caddy（`frontend/vercel.json`；[ADR frontend-on-vercel](../adr/2026-11-03-frontend-on-vercel.md)）。
- `NO_SIDE_EFFECTS` 的方法 client 以 GET 呼叫（connect-es 的 `useHttpGet: true`，見 `frontend/src/lib/api/transport.ts`；connect-go 的 `connect.WithHTTPGet()`），請求編進 query；其他方法是 POST。
- 所有回應都是 `Cache-Control: no-store`（`middleware.go`；`connect_test.go`）。目前沒有 Public 的讀取方法，所以沒有 `public, no-cache` 與 `ETag`（計畫的範圍）。
- 請求 body 上限 1 MiB（`server.go` 的 `maxBodyBytes`；`TestConnectBodyTooLarge`）；模組的文字欄位只檢查產品規格訂的上限。
- 逾時：client 以 `Connect-Timeout-Ms` 帶，前端預設 15 秒（`transport.ts` 的 `defaultTimeoutMs`）；伺服器另有 `http.Server` 的逾時（`server.go` 的 `DefaultTimeouts`）。
- web 用 Connect 的 JSON 格式（connect-web 的預設）。只有 unary 方法：有串流方法的 service，`rpc.NewMux` 拒絕掛載（interceptor 只包 unary，admin 檢查不會執行）。
- 不認得的 JSON 欄位被丟掉（connect-go 的預設），不回錯（ADR api-contract-protobuf-connect Decision）。

## 九、認證與限流

- Session 是 opaque token，只放在 `HttpOnly; Secure; SameSite=Lax` 的 `__Host-session` cookie（不帶 Domain；`COOKIE_SECURE=false` 的本機 http 開發改名為 `session`），DB 只存 SHA-256（ADR api-and-auth-baseline D4）。
- 不接受 Bearer token，也不把 token 放進回應訊息或 JS 讀得到的地方（ADR api-and-auth-baseline D4）。登入一律發新 token。
- 整個 handler 外層包 `http.CrossOriginProtection`，跨來源的狀態變更請求（POST）回 `permission_denied`（`CROSS_ORIGIN_REQUEST`）（`server.go`；ADR api-and-auth-baseline D5；`TestCrossOriginRejectedAsPermissionDenied`）。這就是 GET 只開放給 `Get*`、`List*` 的原因（第三節）。
- 註冊與登入的 per-IP 限流在 net/http 這一層、解碼之前，procedure 列在 `server.go` 的 `connectLimited`，用生成的 procedure 常數（`usersv1connect.UserPublicServiceCreateUserProcedure`）；列了沒掛載的 procedure，server 不會啟動（`TestConnectRateLimitNamesMountedProcedures`）。次數由 `AUTH_RATE_LIMIT_PER_MIN` 設定。
- 登入另有依「帳號＋來源」的節流與帳號層級的上限（`sign_in_throttles`），不論帳號是否存在都記錄，超過時回 `resource_exhausted`（`LOGIN_THROTTLED`，帶 `RetryInfo`）（ADR api-and-auth-baseline D6、ADR account-abuse-and-lifecycle D4）。

### Session 撤銷（新增功能必須遵守）

- 停用帳號時，在同一個 transaction 撤銷該使用者的所有 session（`auth.Service.SetDisabled`，經 `DeleteUserSessions`）。
- **之後新增任何會改變使用者 role 或密碼的功能，必須在同一個 transaction 撤銷該使用者的所有 session**（ADR api-and-auth-baseline D4），並且要有測試：改完後該使用者原有的 cookie 都回 `unauthenticated`（寫法比照 `TestAdminDisablesUserRevokesSessions`）。`app recover-admin` 已照這條做。

## 十、契約穩定性（ADR api-contract-protobuf-connect D1-B）

- `make -C backend api-breaking` 以 `buf breaking` 對 `main`（`BASE`，預設 `origin/main`）的 merge-base 檢查，規則是 `WIRE_JSON` 加 `RPC_NO_DELETE`、`PACKAGE_SERVICE_NO_DELETE`、`PACKAGE_NO_DELETE`（`api/buf.yaml`）；base 沒有 proto 時略過（`scripts/check-buf-breaking.sh`）。
- 還沒進 `main` 的契約在 `dev` 上可以直接改。已進 `main` 的：
  - **可以**新增 service、方法、訊息、欄位、enum 值。
  - **NEVER** 刪除或改名已發布的方法、service、package；**NEVER** 改欄位的編號或型別、刪除欄位而不 `reserved` 它的編號與名稱；JSON 名稱也是契約，欄位改名一樣會破壞 JSON 的 client。
  - 真的要破壞時，在 `api/buf.yaml` 加 ignore，commit 帶 `Quality-Gate:` trailer（ADR api-contract-protobuf-connect D1 的 Rationale）。
- 前後端分開部署（API 在 VPS、前端在 Vercel），部署的空窗期新舊版本會同時存在，這是 breaking 檢查要保護的情況。

## 十一、生成

- proto 改了就重新生成：後端 `make -C backend gen`（`backend/buf.gen.yaml`，輸出 `backend/internal/gen`），前端 `pnpm gen:api`（`frontend/buf.gen.yaml`，輸出 `frontend/src/generated`）。生成物都 commit，**NEVER** 手改；CI 以 `make -C backend gen-check` 與 `pnpm gen:api:check` 擋下沒有重新生成的變更。
- buf 的版本鎖在根目錄 `Makefile`（`make bin/buf`），外掛鎖在 `backend/tools/go.mod` 與 `frontend/package.json`，都在本機執行，不用 remote plugin。
- `make -C backend buf-lint` 檢查 lint。
- Go 的錯誤 details 用 genproto 的 `errdetails`，不自己生成 googleapis；前端從 BSR 釘住的 commit 生成 `google/rpc/error_details.proto`（`frontend/buf.gen.yaml`）。

## 十二、分層與前端呼叫

- Handler 放在 `internal/httpapi`，實作生成的 `<Service>Handler` 介面：轉換生成的訊息、解 ID 與分頁、呼叫 service、把欄位接到請求路徑，不寫業務邏輯，不直接碰 DB。每個 service 以 `rpc.Service{Desc, New}` 描述，列進 `server.go` 的 `connectServices`（`items.go` 的 `itemsService`）。
- `internal/modules/<m>` 只放 service、store 與 sqlc 生成碼；目前使用者經由 `internal/platform/authctx` 傳給 service。模組之間共用、不碰傳輸層的 helper 放在 `internal/platform`（例如 `keyset`）。
- 前端以生成的 service 定義呼叫：讀取用 connect-query 的 `useQuery(Service.method.x, input)`，transport 來自 `TransportProvider`（`frontend/src/providers/query-provider.tsx`）；寫入以 `createClient(Service, transport)` 包在 `features/<name>/api/`，再由 `features/<name>/hooks/` 的 TanStack Query mutation 呼叫（`features/auth/api/auth-api.ts`、`features/auth/hooks/use-auth.ts`）。**NEVER** 在 component 裡直接 `fetch()` 或手寫 procedure 的網址。
- 用到 connect-query 的 hook 檔是 client module（檔頭 `"use client"`，`src/__tests__/client-boundary.test.ts`）。
- 瀏覽器打相對路徑 `/api`；Server Component 用 `frontend/src/lib/api/server-transport.ts` 的 `serverTransport`，打網站自己的 `/api`（`NEXT_PUBLIC_APP_URL`，經 Vercel edge），只轉發自己需要的 header（不轉發 `X-Forwarded-For`、`X-Real-IP`，也不持有 origin secret；ADR frontend-on-vercel）。
- `unauthenticated` 導回登入頁，`INVALID_CREDENTIALS` 除外（`transport.ts` 的 `redirectOnUnauthenticated`）。錯誤文字用 `frontend/src/lib/api/connect-errors.ts` 的 `errorMessage`，欄位錯誤用 `fieldViolations` 與 `violationMessage`。

```typescript
// 正確：frontend/src/features/auth/api/auth-api.ts
const users = createClient(UserPublicService, transport);
const { user } = await users.createUser(credentials);

// 錯誤：手寫網址與型別，繞過生成的契約
await fetch("/api/app.users.v1.UserPublicService/CreateUser", { method: "POST", body: JSON.stringify(credentials) });
```

## 禁止事項

- **NEVER** 先寫 handler 再補 proto；proto 改了沒重新生成（`gen-check`、`gen:api:check` 會擋）
- **NEVER** 在 proto、handler 或前端重寫 service 已有的長度、格式限制（ADR api-contract-protobuf-connect D2）
- **NEVER** 送伺服器產生的使用者文字，或讓前端解析、顯示 `message`（ADR api-contract-protobuf-connect D3）
- **NEVER** 送出 `errors.proto` 沒有宣告的 reason
- **NEVER** 把 `Get*`、`List*` 以外的方法標 `NO_SIDE_EFFECTS`；**NEVER** 把請求帶憑證的讀取標 `NO_SIDE_EFFECTS`
- **NEVER** 新增名稱不是 `PublicService`、`AppService`、`AdminService` 結尾的 service，或串流方法
- **NEVER** 新增 `Create`、`Update` 卻不回傳資源，或沒有對應的 `Get`
- **NEVER** 在 service 決定 Connect code 或寫回應；service 只回 `apperr`
- **NEVER** 查詢使用者擁有的資源而不帶 owner 條件；**NEVER** 對別人的資源回 `permission_denied`
- **NEVER** 用 offset 加總數的分頁（後台頁碼表格除外）
- **NEVER** 接受 Bearer token，或讓 JS 讀得到 session token
- **NEVER** 新增改 role 或改密碼的功能而不撤銷該使用者的所有 session
- **NEVER** 刪除、改名或改型別已進 `main` 的方法或欄位，除非依第十節加 ignore 並帶 `Quality-Gate:`
