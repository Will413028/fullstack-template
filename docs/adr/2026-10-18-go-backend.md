---
title: 後端改寫成 Go：REST＋OpenAPI 契約、pgx＋sqlc＋Atlas、DB opaque session
date: 2026-10-18
review-date: 2027-04-18
status: active
tags: [fullstack-template, decision, go, rest, auth, postgresql]
---

# 後端改寫成 Go：REST＋OpenAPI 契約、pgx＋sqlc＋Atlas、DB opaque session

## Context

這個 repo 是新專案的起點：clone 後 `make rename`，之後與範本斷開。後端原本是 FastAPI（`src/` 1106 行、`tests/` 438 行、4 支 Alembic migration），因為範本最初就是 FastAPI 範本。作者其他在維護的專案已改用 Go modular monolith，從 Python 範本起步的新專案一開始就在錯的 stack 上。前端只呼叫 5 個 `/auth/*` endpoint。

**約束**：

- `external` 範本沒有部署、沒有線上資料：`.github/workflows/` 只有 CI 與 security，沒有 deploy。所以沒有相容性窗口或資料搬移的限制，舊 migration 可以整批換成新的 baseline。
- `external` 範本會被 clone 再改名：Go 的 module path 寫在每一個 import 裡，rename 必須改寫 import，不能只做字串替換。
- `inherited` 前端已依現有 REST 形狀（`{data}` 外殼、`detail`／`errors` 錯誤欄位）實作。現在仍成立的理由：無外部理由；它是我們擁有的程式，只當成「改動量」計，不限制選項。
- `inherited` 與作者其他 Go 專案一致（Protobuf＋Connect、Atlas）。現在仍成立的理由：無外部理由；範本沒有原生 client，一致性不構成外部限制。

## Options Considered

**範圍**

- **基準. 單一後端 stack**：starter 多半只維護一個後端（例如 [full-stack-fastapi-template](https://github.com/fastapi/full-stack-fastapi-template)），多變體採 generator 選項時維護成本倍增。
- **A. 直接取代、Python 刪除並打 tag 保存**（選用）
- **B. rename 時選 Python 或 Go**：兩套後端都要維護、都要測試。

**對外契約**

- **基準. REST＋OpenAPI**：對外與 web API 最普遍的形式，curl 與第三方串接都直覺。
- **A. REST＋OpenAPI spec-first，錯誤 RFC 9457**（選用）
- **B. Protobuf＋Connect（JSON）**：一份 schema 可生成 Go／TS／Swift，buf 內建 breaking 檢查；代價是 web bundle 變大、curl 不直覺、前端呼叫層要重寫。適合有原生 client 的產品。

**資料層與 migration**

- **基準. pgx＋sqlc，搭配 versioned migration（goose／golang-migrate）**：Go 社群主流的型別安全 SQL 做法（[sqlc](https://docs.sqlc.dev/)）。
- **A. pgx＋sqlc＋Atlas 宣告式 schema**（選用）：每個模組一份 `schema.sql`，`atlas migrate diff` 產生 migration。
- **B. GORM 或 ent**：ORM 隱藏 SQL，和「每個查詢都可審閱」的目標相反。

**認證**

- **基準. httpOnly cookie＋server-side opaque session**：第一方瀏覽器 app 的建議做法（[OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)）。
- **A. opaque session，DB 只存 SHA-256，absolute 7 日、idle 1 日**（選用）
- **B. 沿用 access JWT＋refresh token 輪替**

## Decision

- **D1** 範圍：直接取代，舊系統保存在 tag `archive/pre-go-rewrite`。
- **D2** 契約：REST＋OpenAPI 3.0 spec-first（`backend/api/openapi.yaml`），保留現有路徑與 `{data}`／`{data,total,page,size,pages}`／`{detail}` 外殼；錯誤改 `application/problem+json`（RFC 9457），驗證錯誤用擴充成員 `errors`。
- **D3** 資料層：pgx＋sqlc＋Atlas 宣告式，單一 `migrations/` 目錄，squawk 檢查鎖表風險。
- **D4** 認證：opaque session，cookie 名 `session`；`/auth/refresh` 刪除。
- **D5** rename 用 `go mod edit -module` 並改寫 import 前綴，作法同 [`gonew`](https://pkg.go.dev/golang.org/x/tools/cmd/gonew)。

## Rationale

- **D1**：選取代而非並存，因為範本的價值在於「clone 下來就是實際在用的 stack」；兩套後端要雙倍維護。代價：想用 Python 的新專案要從 tag 起步。
- **D2**：選基準 REST 而非 Connect，因為範本沒有原生 client，而 Connect 的主要收益（多語言生成、buf breaking）要有原生 client 才用得到；REST 讓前端幾乎不用改、curl 與第三方串接直覺。代價：跟作者其他 Go 專案的規範不同，breaking 檢查要另接 oasdiff。
- **D3**：選 Atlas 而非基準的 goose，因為 repo 擁有者偏好不手寫 SQL migration，讓 schema 的真相只有 `schema.sql` 一份。代價：產生 diff 需要一個 dev 資料庫（Docker），Atlas 的 migration lint 屬付費功能，所以改用 squawk。sqlc 取代原本的 `BaseRepository[T]` 泛型 CRUD；在 Go 裡泛型 repository 是反模式。
- **D4**：選 session 而非 JWT，因為舊設計本來就每個請求都查 DB（`get_current_user` 解碼 JWT 後再讀 user），refresh token 也存在 DB，JWT 的無狀態優勢從沒用到；換成 session 後可以即時撤銷，前端的 refresh 重試邏輯也能整段刪掉。代價：多一張 `sessions` 表，`last_used_at` 要節流寫入（每分鐘最多一次）。
- **D5**：選改寫 import 而非固定一個中性 module path（例如 `example.com/app`），因為下游專案的 module path 應該是它自己的 repo：`GOPRIVATE`、`go install <path>@latest`、工具輸出裡的套件路徑都以它為準；`gonew` 是 Go 官方為範本 clone 情境提供的做法。代價：`rename.py` 要改寫所有 `.go` 檔並以 `go build` 驗證。

## Expected Outcome

- 新專案 clone、rename 後，`go build ./...`、`go test ./...`、`pnpm build` 直接綠燈，且 repo 裡沒有範本的 module path 殘留。
- 前端只因 `/auth/refresh` 刪除、login 改 JSON 而變動，`ApiError` 的錯誤解析不用改。
- 舊系統與新系統對同一組請求的回應差異，全部屬於刻意列出的變更（改寫計畫的行為等價步驟）。

## Invariants

- service 與 store 不 import `net/http`，錯誤只在 HTTP 邊界轉成 problem+json（depguard）。
- schema 只由 Atlas 從 `schema.sql` 產生；已發布的 migration 只增不改。
- 認證憑證只存在 `HttpOnly` cookie，或由非瀏覽器 client 以 Bearer 送出。

## Revocation Triggers

- 新專案預設會有原生 app → 重新評估 D2（Connect）。
- 需要 MFA、SSO 或第三方登入 → 重新評估 D4（代管認證）。
- backend 要跑超過一個 replica → in-memory rate limit 改用 DB 或 proxy 層。

## Review Notes

## Related

- 來源：repo 擁有者 2026-10-18 與 coding agent 討論當場拍板；本 ADR 即原始紀錄，無外部來源文件。
- 舊系統：tag `archive/pre-go-rewrite`（改寫前的 `main`）。
