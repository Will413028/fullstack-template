# API Design Rules

本專案的 API 是 contract-first：`backend/api/openapi.yaml`（OpenAPI 3.0.3）是 API 形狀與限制的唯一來源，HTTP 層在 `backend/internal/httpapi`，業務邏輯在 `backend/internal/modules/<m>` 的 service。決策來源見 [ADR api-and-auth](../adr/2026-10-18-api-and-auth-baseline.md) 與 [ADR go-backend](../adr/2026-10-18-go-backend.md)。

## 契約是唯一來源

- 新增或修改 API 一律先改 `backend/api/openapi.yaml`，再重新生成：後端 `make -C backend gen`（`internal/httpapi/gen`），前端 `pnpm gen:api`（`frontend/src/lib/api/schema.d.ts`）。CI 以 `make -C backend gen-check` 與 `pnpm gen:api:check` 擋下沒有重新生成的變更（ADR api-and-auth D2）。
- 長度、格式、pattern、enum 只寫在契約裡。後端由 validator（`validate.go` 的 `validateRequests`）執行，前端取用生成的型別。**NEVER** 在 handler、service 或 zod schema 再寫一次相同的限制（ADR api-and-auth D6）。
- 契約表達不了的規則（例如密碼在常見密碼清單內、帳號已被使用）才由 service 檢查，以 `apperr` 回報。
- request body 的 schema 加 `additionalProperties: false`，多送的欄位回 422 `additional_property`。
- DB 的 CHECK 是第二道防線，數值必須和契約一致（見 `database-design.md`）。

```typescript
// 正確：前端只檢查必填，長度與格式交給後端的 field error（frontend/src/features/auth/validations.ts）
export const credentialsSchema = z.object({
  account: z.string().min(1, "required"),
  password: z.string().min(1, "required"),
});

// 錯誤：在前端重寫契約的限制，兩處遲早不一致
account: z.string().min(3).max(50).regex(/^[A-Za-z0-9_.-]+$/),
```

## URL 設計

### 路徑

- API 與前端同源，掛在 `/api` 底下。契約寫 `servers: - url: /api`，`paths` 不含 `/api` 前綴（ADR api-and-auth D1）。
- URL 是資源導向的**複數名詞 kebab-case**：`/users`、`/sessions`、`/items`、`/order-items`（ADR api-and-auth D2）。
- 資源 ID 用路徑參數 `{id}`，型別 `string` + `format: uuid`。
- 「目前的」資源用固定名稱的子資源：`/users/me`、`/sessions/current`。
- 巢狀資源最多兩層：`/users/{id}/items`。
- **NEVER** 在 URL 用動詞：登入是 `POST /sessions`，不是 `POST /users/login`；登出是 `DELETE /sessions/current`。
- **NEVER** 用角色切分路徑（`/admin/users`）：用同一個資源搭配授權檢查。
- 改變資源狀態優先用 PATCH 該資源的欄位，例如停用帳號是 `PATCH /users/{id}` 帶 `is_disabled`。
- URL 不帶版本（ADR api-and-auth D2）。

### Method 與狀態碼

| 操作 | Method | URL | 成功回應 |
|------|--------|-----|---------|
| 建立 | `POST` | `/items` | `201` + `Location` + 資源本身 |
| 列表 | `GET` | `/items` | `200` + `{items, next_cursor}` |
| 取得 | `GET` | `/items/{id}` | `200` + 資源本身 |
| 部分更新 | `PATCH` | `/items/{id}` | `200` + 更新後的資源 |
| 刪除 | `DELETE` | `/items/{id}` | `204`，沒有 body |

- 建立回 201 加 `Location`，刪除回 204（RFC 9110；ADR api-and-auth D2）。
- `Location` 指向契約裡**有 GET** 的正式網址（`/api/items/{id}`），建立者用同一組 cookie GET 得到 200。不可指向別名（例如 `/users/me`）或只有寫入端的路徑（ADR api-and-auth D9）。
- 有 PATCH、PUT 或 DELETE 的 path 一定也有 GET。`TestEveryWritablePathHasGet`（`internal/httpapi/spec_test.go`）檢查契約；整合測試的 `followLocation`（`internal/httpapi/auth_test.go`）對每個 201 GET 它的 `Location`。

```go
// 正確：internal/httpapi/items.go
location := "/api/items/" + it.ID.String()
return gen.CreateItem201JSONResponse{Body: toItem(it), Headers: gen.CreateItem201ResponseHeaders{Location: &location}}, nil
```

## Response 格式

不包外殼（ADR api-and-auth D2）：

- 單一資源直接回 JSON object：`{"id": "…", "title": "…", …}`。
- 集合回 `{<資源複數>: [...], next_cursor}`，例如 `{"items": [...], "next_cursor": "…"}`；最後一頁的 `next_cursor` 是 `null`。
- **NEVER** 回 `{data: …}`、`{detail: "deleted"}` 之類的外殼。
- ID 是 UUID 字串；時間是 RFC 3339 UTC（handler 輸出前呼叫 `.UTC()`）。
- 回應一律帶 `id`；有 `updated_at` 的資源同時回 `created_at` 與 `updated_at`。
- 敏感欄位（`password_hash`、session token 與其雜湊）**NEVER** 出現在任何 response schema。
- 可為 null 的欄位在 response schema 宣告成 required + `nullable: true`，Go 端用 `nullable.Nullable[T]` 並一律設值（`items.go` 的 `optional`），不讓欄位消失。

## 分頁

- 集合一律用 keyset 分頁：query 參數 `limit`（1–100，預設 20）與 `cursor`，引用契約的 `#/components/parameters/Limit`、`Cursor`（ADR api-and-auth D2）。
- `cursor` 是 opaque 字串，由 `httpx.EncodeCursor` 產生（最後一筆 id 的 base64url），client 原樣傳回，**NEVER** 自行組或解析。不合法的 cursor 回 422。
- 不提供總數（不跑 `COUNT(*)`）。排序以 UUIDv7 的 `id DESC`（等於建立時間新到舊），查詢多取一筆判斷是否有下一頁：`httpx.PageRows`、`httpx.TrimPage`。
- 需要頁碼的後台表格才另外用 offset 實作，且只限該 endpoint（ADR api-and-auth D2 Rationale）。
- 契約的 `default` 不會被 validator 填入，handler 的預設值要和契約一致並有測試（`TestDefaultPageSizeMatchesContract`）。

## 部分更新（PATCH）

- PATCH 收 `application/merge-patch+json`（RFC 7396；ADR api-and-auth D2）：沒送的欄位不改，送 `null` 清成 NULL。
- 不可為 null 的欄位在 patch schema 不加 `nullable`，送 `null` 回 422 `null_not_allowed`。
- 可清空的欄位在 Go 端用 `nullable.Nullable[T]` 區分「沒送」與「送 null」，並把「有沒有送」明確傳給 query。

```go
// 正確：internal/httpapi/items.go，SetDescription 區分沒送與 null
p := items.Patch{Title: req.Body.Title, SetDescription: req.Body.Description.IsSpecified(), Description: fromNullable(req.Body.Description)}
```

```sql
-- 正確：internal/modules/items/queries/items.sql
SET title       = coalesce(sqlc.narg('title'), title),
    description = CASE WHEN @set_description::boolean THEN sqlc.narg('description') ELSE description END,
    updated_at  = now()
```

## 錯誤（RFC 9457）

- 所有錯誤回 `application/problem+json`，欄位 `type`、`title`、`status`，選填 `detail`、`code`、`request_id`（ADR api-and-auth D3）。
- `type` 是固定的相對 URI `/problems/<slug>`，每種 problem 一個（`problem.go` 的 `problemType`）。`code` 是細分 `type` 的穩定機器碼，例如 `account_taken`、`insufficient_role`。
- 驗證錯誤是 422 `/problems/validation`，附 `errors: [{pointer, code, detail}]`；`pointer` 是 JSON Pointer（query 或 path 參數為 `/<name>`），`code` 是 `FieldError.code` 的值，例如 `too_short`、`password_too_common`。
- `Problem.type`、`Problem.code`、`FieldError.code` 的可用值在契約裡宣告成 enum。後端由 `TestProblemValuesInContract` 與 `checkProblems`（`problem_contract_test.go`）檢查送出的值都在 enum 內；前端由 `messages.test.ts` 在編譯期檢查 `frontend/messages/*.json` 的 `problems.types`、`problems.codes`、`problems.fields` 涵蓋每個值（ADR api-and-auth D3）。
- 新增 code 的順序：契約 enum → `make -C backend gen` 與 `pnpm gen:api` → 每個語系的 `problems.*` 翻譯 → 後端送出。
- 前端依 `code`、再依 `type` 選訊息（`errors.ts` 的 `problemMessage`、`fieldMessage`），**NEVER** 解析或顯示 `detail`。
- 改 slug 或 code 是 breaking change。

### Service 回報錯誤

- Service 只回傳 `*apperr.Error`（`apperr.New(kind, code, detail)`、欄位錯誤用 `apperr.Invalid(pointer, code, detail)`），kind 有 `NotFound`、`Conflict`、`Unauthenticated`、`Forbidden`、`Validation`、`RateLimited`。
- 轉成 problem 只在 `internal/httpapi` 一處（`problem.go` 的 `writeError` 與 `kindProblems`）。其他 error 一律是 500 `/problems/internal`，內容只寫進 log，**NEVER** 送給 client。
- `detail` 會送給 client：不得含 SQL、主機名稱、secret 或其他使用者的資料。

```go
// 正確：service 回報預期中的失敗
return apperr.New(apperr.Conflict, CodeAccountTaken, "account is taken")

// 錯誤：service 決定 HTTP 狀態碼（modules 也不能 import net/http，depguard 會擋）
http.Error(w, "conflict", http.StatusConflict)
```

## 分層

- Handler 放在 `internal/httpapi`，實作生成的 `gen.StrictServerInterface`：解 request、呼叫 service、轉成生成的 response 型別，不寫業務邏輯，不直接碰 DB。
- `internal/modules/<m>` 只放 service、store 與 sqlc 生成碼，不 import `net/http`、`internal/httpapi`、`cmd`，也不 import 其他模組。`.golangci.yml` 的 depguard 與 `internal/archtest` 檢查（ADR go-backend Invariants）。
- 目前使用者經由 `internal/platform/authctx` 從 `context.Context` 傳給 service。
- 模組之間共用、但不碰 HTTP 的 helper（分頁 cursor）放在 `internal/platform/httpx`。

## 認證

- Session 是 opaque token，只放在 `HttpOnly; Secure; SameSite=Lax` 的 `__Host-session` cookie（不帶 Domain；`COOKIE_SECURE=false` 的本機 http 開發改名為 `session`），DB 只存 SHA-256（ADR api-and-auth D4）。
- 不接受 Bearer token，也不把 token 放進 response body 或 JS 讀得到的地方（ADR api-and-auth D4）。
- 整個 mux 外層包 `http.CrossOriginProtection`（`server.go` 的 `protect`），跨來源的狀態變更請求回 403 `cross_origin_request`（ADR api-and-auth D5）。
- 需要登入的 operation 由契約的 `security: sessionCookie` 宣告；未登入由 validator 回 401。只有登入、註冊、登出這類操作在契約寫 `security: []`。
- 登入一律發新 token。

### Session 撤銷（新增功能必須遵守）

- 停用帳號時，在同一個 transaction 撤銷該使用者的所有 session（`auth.Service.SetDisabled` 呼叫 `DeleteUserSessions`）。
- **之後新增任何會改變使用者 role 或密碼的功能，必須在同一個 transaction 撤銷該使用者的所有 session**（ADR api-and-auth D4）。範本目前沒有這兩個功能，這條規則是給之後新增時遵守的，並且要有測試：改完後該使用者原有的 cookie 都回 401（寫法比照 `TestAdminDisablesUserRevokesSessions`）。

## 授權

- role 只有 `admin` 和 `user`（`auth.RoleAdmin`、`auth.RoleUser`），註冊預設 `user`，admin 只能用 `seed-admin` 建立（ADR api-and-auth D7）。
- 使用者擁有的資源，**每個查詢**都帶 owner 條件（`WHERE id = @id AND owner_id = @owner_id`）；別人的資源回 404，不回 403，不透露它存在（`TestGetOtherUsersItem`）。
- UPDATE 與 DELETE 用 `:one` 或 `:execrows` 回報影響的列，0 列回 404，不可靜默成功。
- 每個 operation 都要在 `internal/httpapi/authz.go` 的 `operationAccess` 寫明誰能呼叫（公開、登入者、admin）；漏列或與契約的 `security: []` 不一致時 server 不會啟動（`TestAdminOperationsChecksTable`）。admin 的 operation 由 `requireRole` 在 handler 執行前回 403 `insufficient_role`（`TestOperationAccessEnforced`）。
- 授權分三處，依「判斷需要什麼」選一處：只看是否登入的寫在契約的 `security`；只看 role 的寫在 `requireRole` 清單；要看呼叫者與資源的關係的（例如 `GET /api/users/{id}` 的「本人或 admin」）寫在 service 或查詢條件，非授權者回 404（`auth.Service.User`、`TestGetOtherUserNotFound`）。
- **NEVER** 先查出資源再在 handler 比對 owner：條件必須在 SQL 裡。

## 限流

- per-IP 限流以 operation 套用：把 operation 加到 `NewHandler` 的 `limitOperations(limiter, limited...)` 清單，次數由 `AUTH_RATE_LIMIT_PER_MIN` 設定。清單用生成的方法名稱（`operationId: createUser` → `CreateUser`），經 `operations(...)` 檢查名稱存在。
- 超限回 429 `/problems/rate-limited` 加 `Retry-After`（`apperr.RateLimited` 的 `RetryAfter`）。
- 登入另有依帳號的節流（`login_throttles`），不論帳號是否存在都記錄（ADR api-and-auth D6）。

## Query 參數

- 參數名用 **snake_case**：`created_after`、`sort_by`。
- 時間一律 RFC 3339 UTC：`2026-03-22T14:30:00Z`；**NEVER** 用 Unix timestamp。
- 分頁只用 `limit`、`cursor`。

## 契約穩定性

- 契約變更必須通過 `make -C backend api-breaking`：oasdiff 以 `main`（`BASE`，預設 `origin/main`）的 merge-base 比對，有 breaking change 就失敗。
- **可以**新增 operation、新增 response 欄位、新增選填的 request 欄位。
- **NEVER** 刪除、改名或改型別已合併的欄位；**NEVER** 把選填的 request 欄位改成必填。
- URL 不帶版本。出現原生 app 或第三方 API 消費者時，重新評估版本策略（ADR api-and-auth Revocation Triggers）。

```yaml
# 正確：新增選填欄位
Item:
  properties:
    title: { $ref: "#/components/schemas/ItemTitle" }
    category: { type: string, nullable: true }

# 錯誤：把 title 改名為 name，前端與已部署的 client 都會壞
Item:
  properties:
    name: { $ref: "#/components/schemas/ItemTitle" }
```

## 前端 API 呼叫

- 一律用 `frontend/src/lib/api/client.ts` 的 `api`（openapi-fetch，型別來自生成的 `paths`），以 `unwrap` 取資料或丟出 `ApiError`。
- 瀏覽器打相對路徑 `/api`；server-side 程式碼打 `API_URL`，只轉發自己需要的 header（不轉發 `X-Forwarded-For`）。
- API 呼叫放在 `features/<name>/api/`，搭配 TanStack Query hooks（`features/<name>/hooks/`）。**NEVER** 在 component 裡直接 `fetch()`。

```typescript
// 正確：frontend/src/features/auth/api/auth-api.ts
import { api } from "@/lib/api/client";
import { unwrap } from "@/lib/api/errors";

export const authApi = {
  register(credentials: Credentials): Promise<User> {
    return unwrap(api.POST("/users", { body: credentials }));
  },
};

// 錯誤：手寫 URL 與型別，繞過生成的契約
await fetch("/api/users", { method: "POST", body: JSON.stringify(credentials) });
```

## 禁止事項

- **NEVER** 先寫 handler 再補契約；契約改了沒重新生成（`gen-check`、`gen:api:check` 會擋）
- **NEVER** 在 handler、service 或前端重寫契約已有的長度、格式、enum 限制
- **NEVER** 回應外殼（`{data}`、`{detail}`）或 offset + 總數的分頁（後台頁碼表格除外）
- **NEVER** 回傳指向沒有 GET 的路徑的 `Location`；**NEVER** 新增有寫入端卻沒有 GET 的 path
- **NEVER** 在 service 決定 HTTP 狀態碼或寫 response；service 只回 `apperr`
- **NEVER** 把非 `apperr` 的錯誤內容送給 client；**NEVER** 讓前端解析 `detail`
- **NEVER** 送出契約 enum 沒有列的 `type` 或 `code`
- **NEVER** 在 URL 用動詞、用角色切路徑或帶版本
- **NEVER** 查詢使用者擁有的資源而不帶 owner 條件；**NEVER** 對別人的資源回 403
- **NEVER** 接受 Bearer token，或讓 JS 讀得到 session token
- **NEVER** 新增改 role 或改密碼的功能而不撤銷該使用者的所有 session
- **NEVER** 刪除、改名或改型別已合併的 API 欄位
- **NEVER** 使用 Unix timestamp，時間一律 RFC 3339 UTC
