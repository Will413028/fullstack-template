# Database Design Rules

本專案使用 PostgreSQL 18、pgx v5（`pgxpool`）、sqlc 與 Atlas 宣告式 schema。schema 與查詢都是可以審閱的 SQL，沒有 ORM。決策來源見 [ADR go-backend](../adr/2026-10-18-go-backend.md)。

## Schema 與查詢的位置

- 每個模組擁有自己的表，宣告在 `backend/internal/modules/<m>/schema.sql`：這份檔案描述表的**目標狀態**，是 schema 唯一的真相（ADR go-backend D2）。
- `backend/atlas.hcl` 收集所有模組的 `schema.sql`，auth 先載入（其他模組的 FK 指向 `users`）。新模組的 `schema.sql` 由 `fileset` 自動納入。
- 所有表都在 `public` schema，DDL 不寫 schema 前綴。
- 查詢只寫在 `internal/modules/<m>/queries/*.sql`，由該模組的 `sqlc.yaml` 生成到 `internal/modules/<m>/internal/store`（`make -C backend sqlc`）。生成碼不手改；`make -C backend sqlc-diff` 擋下過期的生成碼。
- **NEVER** 在 Go 裡組 SQL 字串或用 ORM；每個查詢都經 sqlc 生成（ADR go-backend D2）。
- sqlc 的 uuid 一律 override 成 `github.com/google/uuid.UUID`，`timestamptz` 成 `time.Time`，nullable 欄位生成指標（`emit_pointers_for_null_types: true`）。

## 命名規範

### Table 與 Column

- 表名用**複數 snake_case**：`users`、`sessions`、`login_throttles`、`order_items`。
- 欄位名用 **snake_case**：`created_at`、`owner_id`、`is_disabled`。
- Boolean 欄位用 `is_` / `has_` 前綴：`is_disabled`、`has_paid`。
- 時間欄位用 `_at` 後綴：`created_at`、`last_used_at`、`expires_at`。

### Foreign Key

- 欄位名是 `<被參照的表單數>_id`（`user_id → users.id`），或表達角色的 `<角色>_id`（`owner_id → users.id`）。
- 必須明確寫 `NOT NULL` 或允許 NULL。
- 指向 `users` 的 FK 必須寫明刪除行為：使用者擁有的資料 `ON DELETE CASCADE`（`sessions.user_id`、`items.owner_id`），帳號刪除時一起刪掉；比帳號活得久的參照用 `ON DELETE SET NULL`。沒寫（`NO ACTION`）或 `RESTRICT` 會讓刪除帳號失敗，`TestForeignKeysToUsersAllowDeletion` 擋下。
- 其他 FK 預設不 cascade：刪除仍被參照的 parent 會失敗，而不是靜默刪掉 child；只有純附屬資料才 `ON DELETE CASCADE`。

### Index

- 名稱照 PostgreSQL 自動命名的格式，在 `schema.sql` 明寫：一般索引 `<table>_<columns>_idx`，unique 索引 `<table>_<column>_key`。
- 為 keyset 分頁建複合索引，欄位與查詢的 `WHERE` 和 `ORDER BY` 一致。

```sql
-- 正確：internal/modules/items/schema.sql
-- Keyset pagination: WHERE owner_id = $1 AND id < $cursor ORDER BY id DESC.
CREATE INDEX items_owner_id_id_idx ON items (owner_id, id DESC);

CREATE UNIQUE INDEX sessions_id_key ON sessions (id);
```

## 模組邊界

- 跨模組只允許 FK 指向 `users`（ADR go-backend Invariants）。
- 模組的查詢**NEVER** `SELECT` 或 `JOIN` 其他模組的表。需要別的模組的 schema 才能解析 FK 時，`sqlc.yaml` 的 `schema` 可以列出它（items 列出 `../auth/schema.sql`），並設 `omit_unused_structs: true`，但查詢仍不讀那些表。
- 需要別的模組的資料時，經由該模組的 service 取得。

## 型別選擇

照 [PostgreSQL wiki「Don't Do This」](https://wiki.postgresql.org/wiki/Don%27t_Do_This)（ADR go-backend D3）：

| 場景 | 使用 | 避免 |
|------|------|------|
| 主鍵 | `uuid PRIMARY KEY DEFAULT uuidv7()` | `serial`、`int` 自增 |
| 文字（有長度限制） | `text` + `CHECK (char_length(x) BETWEEN m AND n)` | `varchar(n)`、`char(n)` |
| 文字（無限制） | `text` | `varchar` |
| 狀態、role、列舉 | `text` + `CHECK (x IN (...))` | PG `ENUM` 型別、沒有 CHECK 的 `text` |
| 時間 | `timestamptz` | `timestamp`（不帶時區） |
| 整數計數 | `bigint` | `int`（squawk `prefer-bigint-over-int`） |
| 金額 | `numeric(precision, scale)` | `float`、`real`、`money` |
| 布林 | `boolean` | `smallint`、`'Y'/'N'` |
| 二進位（雜湊等） | `bytea` + `CHECK (octet_length(x) = n)` | hex 字串存 `text` |

- 主鍵由 DB 的 `uuidv7()`（PostgreSQL 18 內建）產生，JSON 裡是字串。UUIDv7 依時間排序，可以直接當 keyset 分頁的排序鍵（ADR go-backend D3）。
- 長度與 enum 的 CHECK 數值必須和 `backend/api/openapi.yaml` 一致：契約是第一道防線，CHECK 是第二道，任何不合長度或 enum 的資料在 DB 層就寫不進去（ADR go-backend Expected Outcome）。
- 新增狀態值時，在同一個 PR 改契約的 `enum` 與 `schema.sql` 的 CHECK。

```sql
-- 正確：internal/modules/auth/schema.sql
CREATE TABLE users (
  id            uuid        PRIMARY KEY DEFAULT uuidv7(),
  account       text        NOT NULL UNIQUE
                            CHECK (account = lower(account) AND char_length(account) BETWEEN 3 AND 50),
  password_hash text        NOT NULL,
  role          text        NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
  is_disabled   boolean     NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- 錯誤：長度寫在型別裡、狀態沒有 CHECK、連號主鍵
CREATE TABLE orders (
  id     serial PRIMARY KEY,
  title  varchar(200),
  status text
);
```

### JSONB 與 Array

- JSONB 只用於彈性結構的資料（偏好設定、外部系統的原始 payload）；使用前必須能說明為何不用 table + FK。**NEVER** 在 JSONB 塞可以正規化的關聯資料。
- 需要查詢 JSONB 或 array 的內容時，建 GIN 索引。
- 簡單的多值屬性（標籤）可以用 `text[]`，不用 JSON 存字串列表。

## 時間戳記

- 對外的資源表（API 回傳 `updated_at` 的）都有 `created_at`、`updated_at timestamptz NOT NULL DEFAULT now()`。
- **這些表的每個 UPDATE 查詢都明寫 `updated_at = now()`**，不靠 trigger，也不由 Go 傳值；並有測試覆蓋（`TestUpdatedAtChanges`）（ADR go-backend D3）。
- 例外：內部的狀態表可以由 service 傳入時間，讓測試控制時鐘（`login_throttles.updated_at` 與 `sessions.last_used_at` 由 `@now` 傳入，限流與 idle 期限的測試因此不用等真實時間）。
- 連線的 `TimeZone` 固定為 UTC。

```sql
-- 正確：internal/modules/auth/queries/users.sql
UPDATE users
SET is_disabled = @is_disabled, updated_at = now()
WHERE id = @id
RETURNING ...;
```

## Migration

### 產生

- 改 `schema.sql` 後，用 Atlas 產生 migration：`make -C backend migration name=<name>`，或在 root `make generate-migration MSG=<name>`（需要 Docker）。產生的檔案在 `backend/migrations/`，連同 `atlas.sum` 一起 commit。
- 套用：root `make migration`（compose 的 `migrate` service 執行 `app migrate`）。
- 產生 migration 與漂移檢查用 Atlas standard 版（`make -C backend tools` 下載到 `bin/atlas`）；image 內套用用 community 版（ADR go-backend D2）。
- `make -C backend drift` 檢查 `schema.sql` 與 `migrations/` 一致。
- **NEVER** 手寫 DDL migration。例外只有兩種，都在 `backend/` 下用 `bin/atlas migrate new <name> --env dev` 建立後手寫，一樣經 squawk、drift 與 immutable 檢查（ADR go-backend D2）：
  - 純資料的 backfill（對既有表走 expand 步驟時）。
  - Atlas 產生的 DDL 會被 squawk 擋下（掃表時持有阻擋寫入的鎖）時，改寫成不鎖表的步驟：先改 `schema.sql` 並產生 migration，刪掉產生的檔案（`bin/atlas migrate hash --env dev`），再手寫等價的步驟，`make drift` 證明結果與 `schema.sql` 相同。例：FK 先 `ADD CONSTRAINT … NOT VALID`，下一支 `VALIDATE CONSTRAINT`（`migrations/*_items_owner_cascade_*`）。

### 不可修改

- 已合併進 `main` 的 migration 只能新增，**NEVER** 修改、刪除或改名；要改就新增一支。`make -C backend migrations-immutable` 以 `main`（`BASE`，預設 `origin/main`）的 `atlas.sum` 為基準檢查（ADR go-backend D2）。
- 還沒合併進 `main` 的 migration 可以刪掉重新產生。

### 鎖與安全（squawk）

- `make -C backend squawk` 用 squawk 檢查每支 migration 會不會長時間鎖表（設定在 `backend/.squawk.toml`）。Atlas 的 migration lint 是付費功能，所以用 squawk（ADR go-backend D2）。
- 對既有的表建或刪索引一律 `CONCURRENTLY`：`atlas.hcl` 的 `concurrent_index` 讓 Atlas 自動產生，並在檔頭加 `-- atlas:txmode none`。
- 對已有資料的表加帶常數 default 的 `NOT NULL` 欄位（`ADD COLUMN n bigint NOT NULL DEFAULT 0`）是安全的：PostgreSQL 11 起不重寫表，squawk 也放行。會重寫表或讓既有列違反約束的是 volatile default（例如 `DEFAULT uuidv7()`）與沒有 default 的 `NOT NULL` 欄位，squawk 會擋下。這兩種改走 expand 步驟，每一步是一支 migration：先在 `schema.sql` 加 nullable、沒有 default 的欄位並產生 migration → `atlas migrate new` 手寫 backfill（ADR go-backend Review Notes）。需要 default 時，backfill 之後再在 `schema.sql` 加上：`ALTER COLUMN … SET DEFAULT` 只影響之後的新列，不重寫表。
- 對已有資料的表補 `NOT NULL` 或加 FK、CHECK：Atlas 產生不了 `NOT VALID`（v1.3.3 實測：`schema.sql` 寫了 `NOT VALID`，產生的仍是一般的 `ADD CONSTRAINT`），照上面的例外手寫不鎖表的步驟。`NOT NULL` 的步驟是 `CHECK (x IS NOT NULL) NOT VALID` → 下一支 `VALIDATE CONSTRAINT` → 再下一支 `SET NOT NULL` 並 `DROP CONSTRAINT` 那個 CHECK（PostgreSQL 12 起 `SET NOT NULL` 會用已驗證的 CHECK 跳過掃表）。
- **NEVER** 在 migration 裡寫 `SET lock_timeout`：`app migrate` 已在連線上設定 `lock_timeout` 5s（`internal/platform/migrate`），每支檔案在自己的 transaction 裡執行。

## DB 角色與權限

照 OWASP 最小權限（ADR go-backend D5）：

- `POSTGRES_USER`（superuser）只用於初始化與容器內維運，app **NEVER** 用它連線。
- migrate 用 owner 角色（`DB_OWNER_USER`，例如 `fullstack_owner`）：擁有 database 與 `public` schema，不是 superuser，經 `MIGRATE_DATABASE_URL` 連線。
- `serve` 與 `seed-admin` 用 app 角色（`DB_APP_USER`，例如 `fullstack_app`），只有 DML，經 `DATABASE_URL` 連線。
- 角色由 `deploy/postgres/initdb/10-roles.sh` 建立；授權（含 `ALTER DEFAULT PRIVILEGES`）由 `app migrate` 每次套用後冪等執行，所以新表自動給 app 角色 DML。
- 密碼不放在 URL 或環境變數：用 `*_FILE`（compose secrets 掛檔）傳入，URL 只寫使用者名稱。
- 整合測試（testcontainers，PostgreSQL 18）使用同一套角色。

## 連線設定

`internal/platform/db` 的 `Open` 建立 `pgxpool`，設定只有以下這些：

| 設定 | 來源 | 預設 |
|------|------|------|
| `MaxConns` | `DB_MAX_CONNS` | 20 |
| `statement_timeout` | `DB_STATEMENT_TIMEOUT` | 5s |
| `lock_timeout` | 常數 | 3s |
| `idle_in_transaction_session_timeout` | 常數 | 30s |
| `TimeZone` | 常數 | UTC |

- 這些以 `RuntimeParams` 設在每條連線上，讓失控的查詢、排隊等鎖或忘了結束的 transaction 佔不住連線。
- pool 是 lazy 連線：DB 暫時連不上不會讓啟動失敗，由 readiness（`/health/ready`）回報。
- 所有服務的 `DB_MAX_CONNS` 總和不得超過 PostgreSQL 的 `max_connections`。
- migrate 連線另設 `lock_timeout` 5s，單次套用上限 30 分鐘（`internal/platform/migrate`）。

## Transaction

- 需要多個語句原子完成的 service 方法，自己用 `pgx.BeginFunc` 開 transaction，並以 `store.New(tx)` 取得查詢。單一語句不用 transaction。
- **NEVER** 在 transaction 內呼叫外部服務（HTTP、HIBP、第三方 API）或做耗時的 CPU 工作（密碼雜湊）：transaction 持有連線與鎖，回應時間不可控會耗盡 pool，也會撞上 `idle_in_transaction_session_timeout`。先在 transaction 外做完，再開 transaction 寫入。
- 需要在 commit 之後才做的事（通知、webhook），放在 `BeginFunc` 回傳成功之後。

```go
// 正確：internal/modules/auth/auth.go 的 CreateUser，雜湊在 transaction 外
hash, err := s.newPasswordHash(ctx, password)
if err != nil {
	return User{}, Session{}, err
}
err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
	q := store.New(tx)
	row, err := q.CreateUser(ctx, store.CreateUserParams{Account: accountKey(account), PasswordHash: hash, Role: RoleUser})
	// ...
	sess, err = s.newSession(ctx, q, u.ID)
	return err
})

// 錯誤：transaction 內呼叫外部服務
err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
	if _, err := store.New(tx).CreateOrder(ctx, params); err != nil {
		return err
	}
	return s.notifier.Send(ctx, userID, "order created") // 禁止
})
```

## 禁止事項

- **NEVER** 手寫 DDL migration（不鎖表的改寫與 backfill 例外），或修改、刪除、改名已合併進 `main` 的 migration
- **NEVER** 繞過 sqlc：在 Go 裡組 SQL 字串、用 ORM、手改生成碼
- **NEVER** 在模組的查詢讀其他模組的表；**NEVER** 新增指向 `users` 以外的跨模組 FK
- **NEVER** 用 `serial` 或 `int` 自增當主鍵；主鍵一律 `uuid DEFAULT uuidv7()`
- **NEVER** 用 `varchar(n)`；有長度限制用 `text` + CHECK，數值與契約一致
- **NEVER** 用 PG `ENUM` 型別，或讓狀態欄沒有 `CHECK (x IN (...))`
- **NEVER** 存不帶時區的時間；**NEVER** 用 `float` 存金額
- **NEVER** 對資源表寫一個不設 `updated_at = now()` 的 UPDATE 查詢
- **NEVER** 對既有的表直接加 volatile default 或沒有 default 的 `NOT NULL` 欄位，或不用 `CONCURRENTLY` 建索引
- **NEVER** 讓 app 用 owner 或 superuser 角色連線；**NEVER** 把 DB 密碼寫進 URL
- **NEVER** 在 JSONB 中塞可以正規化的關聯資料
- **NEVER** 在 DB transaction 內呼叫外部服務或做密碼雜湊這類耗時工作
