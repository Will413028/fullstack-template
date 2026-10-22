---
title: 後端改寫成 Go：直接取代、pgx＋sqlc＋Atlas、PostgreSQL 型別基準
date: 2026-10-18
review-date: 2027-04-18
status: active
tags: [fullstack-template, decision, go, postgresql, migration]
---

# 後端改寫成 Go：直接取代、pgx＋sqlc＋Atlas、PostgreSQL 型別基準

## Context

這個 repo 是新專案的起點。clone 下來後跑 `make rename`，之後就和範本斷開，所以範本裡的每一個預設值都會變成每個新專案的預設。

後端原本是 FastAPI：`src/` 1106 行、`tests/` 438 行、4 支 Alembic migration。當初選 FastAPI，是因為範本一開始就是 FastAPI 範本。作者其他在維護的專案都已改用 Go modular monolith，如果範本留在 Python，新專案一開始就站在錯的 stack 上。

API 形狀、認證與部署拓樸另見 [API 與認證基準](2026-10-18-api-and-auth-baseline.md)。

**約束**：

- `external`：範本沒有部署，也沒有線上資料（`.github/workflows/` 只有 CI 與 security）。所以沒有相容性窗口要顧，也不用搬資料，舊 migration 可以整批換成新的 baseline。
- `external`：範本會被 clone 再改名。Go 的 module path 會出現在每一個 import 裡。
- `inherited`：`docs/rules/database-design.md` 規定 `String(N)`、狀態欄不加 CHECK、主鍵用 int 自增。這條限制現在仍成立的理由：沒有外部理由。這份規則是我們自己寫的，本次一併重訂。

## Options Considered

**範圍**

- **基準**：starter 只維護一個後端 stack，例如 [full-stack-fastapi-template](https://github.com/fastapi/full-stack-fastapi-template)。
- **A. 直接取代**（選用）：刪除 Python，用 tag 保存舊版。
- **B. rename 時選 Python 或 Go**：兩套後端都要維護、都要測試。

**資料層與 migration**

- **基準**：pgx＋sqlc（[sqlc](https://docs.sqlc.dev/)），搭配 versioned migration（goose 或 golang-migrate）。
- **A. pgx＋sqlc＋Atlas 宣告式 schema**（選用）：每個模組一份 `schema.sql`，由 `atlas migrate diff` 產生 migration。
- **B. GORM 或 ent**：ORM 把 SQL 藏起來，和「每個查詢都可以審閱」的目標相反。

**型別與主鍵**

- **基準**：依 [PostgreSQL wiki「Don't Do This」](https://wiki.postgresql.org/wiki/Don%27t_Do_This)，用 `text`（不用 `varchar(n)`）、`timestamptz`、identity（不用 `serial`）。對外的 ID 是 opaque 字串（[Zalando #174](https://opensource.zalando.com/restful-api-guidelines/)），使用者 ID 不可預測（[OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)）。
- **A. `uuid DEFAULT uuidv7()`；`text` 加 CHECK；狀態欄 `text` 加 CHECK**（選用）。`uuidv7()` 是 [PostgreSQL 18 內建函式](https://www.postgresql.org/docs/18/functions-uuid.html)。
- **B. `bigint` identity 以字串輸出**：比較小，但會透露資料量，也容易被猜到。
- **C. 沿用現行規則**：`varchar(N)`、int 主鍵、狀態欄不加 CHECK。

## Decision

- **D1**：直接取代，舊系統保存在 tag `archive/pre-go-rewrite`。
- **D2**：pgx＋sqlc＋Atlas 宣告式 schema。只有一個 `migrations/` 目錄；用 squawk 檢查會鎖表的變更；合併進 `main` 的 migration 只能新增，不能修改，還沒合併的可以重新產生（2026-10-18 修訂）。DDL 只由 `atlas migrate diff` 產生；兩種例外用 `atlas migrate new` 手寫，一樣經 squawk、drift 與 immutable 檢查：純資料的 backfill（對既有表走 expand 步驟時），以及 Atlas 產生的 DDL 被 squawk 判定會鎖表時改寫成的不鎖表步驟（2026-10-22 修訂）。Atlas 分兩個版本別（2026-10-18 修訂，見 Review Notes）：產生 migration 與漂移檢查用 standard 版，只在開發機與 CI；image 內套用 migration 用 community 版。兩者都鎖同一個版本與 checksum（2026-10-18 起 v1.3.3）。
- **D3**：主鍵用 `uuid DEFAULT uuidv7()`，JSON 裡是字串。文字欄用 `text`，有長度限制時加 CHECK；狀態與 role 欄用 `text` 加 `CHECK (x IN (...))`；時間一律用 `timestamptz`。`updated_at` 由每一個 UPDATE 查詢明寫 `= now()`，並有測試覆蓋。
- **D4**：`make rename` 先執行 `go mod edit -module`，再改寫所有 import 前綴，預設路徑是 `github.com/Will413028/<NAME>/backend`，也可以用 `MODULE=` 覆寫。作法同 [`gonew`](https://pkg.go.dev/golang.org/x/tools/cmd/gonew)。

- **D5 DB 角色與憑證**（2026-10-18 追加）：`POSTGRES_USER`（superuser）只用於初始化與容器內維運；migrate 用 `<app>_owner`（擁有 database 與 `public` schema，非 superuser）；`serve` 與 `seed-admin` 用 `<app>_app`（只有 DML）。角色由 `docker-entrypoint-initdb.d` 腳本建立，授權由 `app migrate` 在每次套用後冪等執行（含 `ALTER DEFAULT PRIVILEGES`）。密碼以 compose secrets 掛檔，經 `*_FILE` 讀取，不放在 URL 或環境變數。整合測試使用同一套角色。

## Rationale

- **D1**：範本的價值在於 clone 下來就是作者實際在用的 stack。兩套後端並存，維護與測試成本都會加倍。代價：想用 Python 的新專案，要從 tag 起步。
- **D2**：選 Atlas 而不選基準的 goose，因為 repo 擁有者偏好不手寫 SQL migration，讓 `schema.sql` 成為 schema 唯一的真相。代價有兩個：產生 diff 需要一個 dev 資料庫（Docker）；Atlas 的 migration lint 屬於付費功能，所以改用 squawk。community 版（Apache-2.0）不接受多個 `schema.sql` 當 `src`，`concurrent_index` 也不加 `-- atlas:txmode none`（FathomPod v1.1.3 實測），所以產生端用 standard 版：免費、不需登入，但授權是 EULA，不是開源；從範本 clone 出去的專案，開發時也會用到它。套用端兩版都遵守 txmode directive，正式 image 用開源的 community 版。不選「全用 community 版、自寫合併 schema 與補 directive 的腳本」，因為那兩支腳本要長期維護，且與 FathomPod 的做法分岔。sqlc 取代原本的 `BaseRepository[T]` 泛型 CRUD；在 Go 裡，泛型 repository 是反模式。
- **D3**：選 UUIDv7 而不選 bigint，因為範本會被拿去做對外服務。連號 ID 會洩漏資料量，也讓人可以猜 ID；UUIDv7 依時間排序，對 B-tree 索引友善，而且 PG18 與 Go 1.27 都原生支援。代價：每個 ID 16 bytes，而且會透露建立時間到毫秒。狀態欄加 CHECK，推翻了舊規則的理由「只有這個 app 寫 DB」。資料完整性應該由資料庫保證，在 Atlas 宣告式 schema 下改 CHECK 只是一行 schema 的修改，舊規則擔心的「兩處要同步」成本很低。
- **D4**：選擇改寫 import，而不固定用一個中性的 path（例如 `example.com/app`），因為下游專案的 module path 應該是它自己的 repo：`GOPRIVATE`、`go install <path>@latest`、工具輸出的套件路徑都以它為準。代價：`rename.py` 要改寫所有 `.go` 檔，並用 `go build` 驗證。

- **D5**：OWASP Database Security Cheat Sheet 要求應用程式帳號只有最小權限，而且「should not be the owner of the database」。舊系統共用 superuser 不是技術前提：它本來就有獨立的 migrate service，只是沿用 postgres image 的預設帳號與一份共用的 `.env`。授權放在每次 migrate，而不是只寫在 initdb，因為 initdb 只在空 volume 執行，`ALTER DEFAULT PRIVILEGES` 也只影響之後由該角色建立的物件（PostgreSQL 文件）；這樣權限不依賴 volume 是用哪一版腳本初始化的。`*_FILE` 是 Docker Official Images 的慣例，Docker 文件指出用環境變數傳密碼有外洩風險。不選「維持單一 superuser 並寫進文件」：app 被打穿時等於整個 DB 被拿下，而且每個下游專案都會繼承。代價：多一個 `app migrate` 子指令、config 要支援 `_FILE`、`.env-example` 多幾個變數。

## Expected Outcome

- 新專案 clone 並 rename 後，`go build ./...`、`go test ./...`、`pnpm build` 直接綠燈，repo 裡不殘留範本的 module path。
- 任何不符合長度、enum 的資料，在 DB 層就寫不進去。
- 舊系統的每一個業務行為，在新系統都有對應的測試或刻意的變更（改寫計畫的步驟 7）。

## Invariants

- service 與 store 不 import `net/http`（depguard）。
- schema 只由 Atlas 從 `schema.sql` 產生（純資料 backfill 例外）；合併進 `main` 的 migration 只能新增，不能修改。
- 跨模組只允許 FK 指向 `users`。

## Revocation Triggers

- Atlas 的 standard 版改成需要登入或付費、或下架週期短到每次升版都來不及重測，或 community 版的 `migrate apply` 不再遵守 txmode directive → 重新評估 D2（改用 goose，或全用 community 版加合併腳本）。
- 某個下游專案用 managed Postgres（initdb 不會執行）→ D5 的角色改由 runbook 以 superuser 建立，授權仍由 `app migrate` 執行。
- 某個下游專案需要可排序、且不洩漏時間的 ID → 重新評估 D3（改用 UUIDv4 或 bigint）。

## Review Notes

- 2026-10-18（Will 選 A）：Atlas 會把發布超過 6 個月的二進位從 CDN 移除，v1.1.3 已在下架邊緣，改鎖 v1.3.3，並以 `make atlas-age` 在發布滿 150 天時擋下 CI。v1.3.3 兩個版本別的多檔 `src`、txmode directive 與套用行為已在隔離容器重測。代價：每 5 個月內要升一次版，Dependabot 追不到。

- 2026-10-18（Will，照建議）：原 revocation trigger「community 版拿掉必要功能」在計畫步驟 0 查證時已成立：community 版 v1.1.3 不接受多個 schema 檔（實測）。依 FathomPod 的實測與做法，D2 改為 standard 版產生、community 版套用，不改用 goose。來源：wiki `tech/atlas-migrate-ci-gate` Case 25；FathomPod ADR `2026-10-07-go-rewrite-scope-and-cutover`。

- 2026-10-18（Will 照建議，計畫 D17）：給既有的 sessions 表加 `id … DEFAULT uuidv7()` 被 squawk 擋下（volatile default 會重寫表）。D2 的不可變規則原本寫「baseline 之後」，但它要保護的是已經套用過這些 migration 的資料庫，而合併前不存在這樣的資料庫：`origin/main` 沒有 migration、範本沒有部署。所以把 PR #12 上未發布的 migration 重新產生成一支 baseline，規則改以「合併進 `main`」為界，由 `migrations-immutable` 以 `main` 為基準守住。合併後對既有表加 volatile default 或 NOT NULL 時要走 expand 步驟，其中的資料 backfill 是 diff 產生不了的，所以「不手寫」改為只限 DDL。

- 2026-10-19（Will 照建議，收尾的設計審查）：HTTP 層用 oapi-codegen strict server 加 kin-openapi validator，每個 request 會被解析兩次，兩邊對同一份輸入的判斷不完全相同（例如 binder 拒收 kin-openapi 放行的 `0x10`）。`internal/httpapi` 的 `validate.go` 與 `bind.go` 合計 353 行（含 validator 本身的掛載），大半是把兩邊的錯誤統一成同一種 422 problem 的轉接碼。只解析一次的替代做法是 [ogen](https://github.com/ogen-go/ogen)，沒有實測。決定保留現狀：這層已經實作完成，並有測試與 mutation 守住；ogen 未經實測，現在換等於重寫 HTTP 層。這不是跨專案一致性的選擇：FathomPod 與 dailyfresh 的契約是 Protobuf＋Connect，沒有用 oapi-codegen。代價是這層轉接碼要跟著兩個套件的行為維護。重新評估的觸發（任一）：轉接層要再加一類新的特例；下一個衍生專案開工前，先做 ogen 的 spike。

- 2026-10-22（Will 選修訂）：`items.owner_id` 改成 `ON DELETE CASCADE`（刪除帳號）時，Atlas 產生的是一般的 `ADD CONSTRAINT … FOREIGN KEY`，squawk 擋下：它在驗證既有列時持有阻擋寫入的鎖。不鎖表的寫法 `NOT VALID` 加下一支 `VALIDATE CONSTRAINT` 是 Atlas 產生不了的（v1.3.3），原本的規則要求「遇到時先修訂本 ADR」。不選「接受鎖表並在檔案加 squawk 忽略」，因為衍生專案遇到有資料的表時仍無做法可循。修訂後允許手寫這類改寫，條件是 squawk 原本會擋、`make drift` 證明結果與 `schema.sql` 相同。`schema.sql` 仍是 schema 唯一的真相。

## Related

- 來源：repo 擁有者於 2026-10-18 與 coding agent 討論後當場拍板（D6 第二次拍板「照業界基準」）。本 ADR 即原始紀錄，沒有外部來源文件。
- 舊系統：tag `archive/pre-go-rewrite`。
- 同批決定：[API 與認證基準](2026-10-18-api-and-auth-baseline.md)。
