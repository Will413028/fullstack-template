# Database Design Rules

本專案使用 async SQLAlchemy 2.0 + asyncpg + Alembic。所有資料庫設計必須遵守以下規則。

## 命名規範

### Table

- 名稱用**複數 snake_case**：`users`, `order_items`, `payment_records`
- 使用 `__tablename__` 明確指定，不依賴自動推導

```python
# 正確
class OrderItem(Base, TimestampMixin):
    __tablename__ = "order_items"

# 錯誤
class OrderItem(Base):  # 缺少 __tablename__，也缺少 TimestampMixin
    pass
```

### Column

- 欄位名用 **snake_case**：`created_at`, `user_id`, `is_active`
- Boolean 欄位用 `is_` / `has_` 前綴：`is_active`, `is_verified`, `has_paid`
- 時間欄位用 `_at` 後綴：`created_at`, `deleted_at`, `expired_at`

### Foreign Key

- 格式：`<referenced_table_singular>_id`
- 例如：`user_id` → `users.id`，`order_id` → `orders.id`
- 必須明確是否允許 NULL
- 預設 **不使用 CASCADE DELETE**，除非是純附屬資料（如 log、子項）

```python
# 一般 FK：不 cascade，刪除 parent 前須先處理 child
owner_id: Mapped[int] = mapped_column(ForeignKey("users.id"))

# 純附屬資料：可以 cascade（刪除 order 連帶刪除 order_items）
order_id: Mapped[int] = mapped_column(ForeignKey("orders.id", ondelete="CASCADE"))
```

### Index

- 一般索引命名：`idx_<table>_<column>`
- Unique 索引命名：`uidx_<table>_<column>`
- 複合索引要在 `__table_args__` 中用 `Index()` 定義，名稱包含所有欄位

```python
from sqlalchemy import Index

class Order(Base, TimestampMixin):
    __tablename__ = "orders"
    # ...
    __table_args__ = (
        Index("idx_orders_user_id", "user_id"),
        Index("uidx_orders_order_no", "order_no", unique=True),
        Index("idx_orders_status_created", "status", "created_at"),
    )
```

## Model 必要結構

### 每個 Model 必須

1. 繼承 `Base` + `TimestampMixin`（提供 `created_at`, `updated_at`）
2. 明確定義 `__tablename__`
3. 使用 `Mapped[]` + `mapped_column()` 型別宣告（SQLAlchemy 2.0 風格）
4. 用 `Mapped[]` 的型別控制 nullable（`Mapped[str]` = NOT NULL，`Mapped[str | None]` = NULL），不需要再加 `nullable=` 參數

```python
from sqlalchemy import ForeignKey, String, Text
from sqlalchemy.orm import Mapped, mapped_column

from src.core.models.base import Base, TimestampMixin


class Item(Base, TimestampMixin):
    __tablename__ = "items"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    title: Mapped[str] = mapped_column(String(200))               # NOT NULL（由 Mapped[str] 推導）
    description: Mapped[str | None] = mapped_column(Text)          # NULL（由 Mapped[str | None] 推導）
    owner_id: Mapped[int] = mapped_column(ForeignKey("users.id"))  # NOT NULL
```

### SoftDeleteMixin

- 需要軟刪除時才加 `SoftDeleteMixin`，不要預設加在所有 table 上
- 使用軟刪除的 table，repository 查詢必須過濾 `deleted_at IS NULL`

### 主鍵

- 預設使用 `int` 自增主鍵
- 若有分散式需求再考慮 UUID，但必須用 `postgresql.UUID` 型別，不要用 String 存

## 型別選擇

| 場景 | 使用 | 避免 |
|------|------|------|
| 短文字（名稱、標題） | `String(N)` 指定長度 | 無限制 `String` |
| 長文字（描述、內容） | `Text` | `String(10000)` |
| 時間 | `DateTime(timezone=True)` | 不帶 timezone 的 DateTime |
| 金額 | `Numeric(precision, scale)` | `Float` |
| JSON 資料 | `postgresql.JSONB` | `JSON` 或 `Text` 存 JSON string |
| 布林 | `Boolean` | `SmallInteger` |
| 列舉/狀態 | `String` + Pydantic Enum 驗證 | PG `Enum()` |
| 陣列（標籤、多值） | `postgresql.ARRAY(Text)` | JSON 存簡單字串列表 |

### Enum / 狀態欄位規範

DB 層用 `String`，驗證交給 Pydantic Enum。不使用 PG Enum，也不加 `CheckConstraint`。

**為什麼不用 PG Enum：**
- `ALTER TYPE ADD VALUE` 不能 rollback，migration 風險高
- 刪除/重命名值幾乎不可能
- 每次加狀態都要 migration，不值得

**為什麼不加 CheckConstraint：**
- 每次加狀態要改兩個地方（Python enum + DB constraint），遲早忘記同步
- Pydantic 已在 API 入口驗證，無效值進不了 service 層
- 單體應用只有這個 app 寫 DB，不存在繞過驗證寫入的場景
- 加 migration 只為了改 constraint = over-engineering

**正確做法：** DB 存 `String`，Pydantic `str, enum.Enum` 做驗證：

```python
import enum
from sqlalchemy import String

# schemas.py — Pydantic 層驗證（API 入口把關，唯一的驗證點）
class OrderStatus(str, enum.Enum):
    PENDING = "pending"
    CONFIRMED = "confirmed"
    SHIPPED = "shipped"
    CANCELLED = "cancelled"

# models.py — DB 層只存 String，不加約束
class Order(Base, TimestampMixin):
    __tablename__ = "orders"
    status: Mapped[str] = mapped_column(String(20), default="pending")
```

新增狀態只需改 Python enum，不需要任何 DB migration。

**何時才需要 DB 層約束：** 多個服務共用同一個 DB、或有人會直接操作 DB 時，再加 `CheckConstraint`。

### JSONB 使用規範

JSONB 適用於**彈性結構資料**（如商品 attributes、使用者偏好設定），但不可取代正規化設計。

- 使用前必須能說明為何不用 table + FK
- 需要查詢 JSONB 內容時，建立 GIN 索引
- 欄位預設值用 `server_default=text("'{}'::jsonb")`

```python
from sqlalchemy.dialects.postgresql import JSONB

attributes: Mapped[dict] = mapped_column(JSONB, server_default=text("'{}'::jsonb"))
```

### Array 使用規範

適用於標籤、多值簡單屬性、非關聯資料：

```python
from sqlalchemy.dialects.postgresql import ARRAY

tags: Mapped[list[str]] = mapped_column(ARRAY(Text), server_default=text("'{}'::text[]"))
```

需要查詢時建立 GIN 索引。

## Relationship

- 使用 `relationship()` 搭配 `back_populates`，不用 `backref`
- **Async 模式下禁止隱式 lazy loading** — 存取未 eager load 的 relationship 會直接噴 `MissingGreenlet` error，不是 lazy load
- 必須在查詢中**明確**使用 `selectinload()` / `joinedload()` 載入關聯資料

```python
# 正確：在 model 定義 relationship（宣告關聯，不控制載入策略）
class User(Base, TimestampMixin):
    __tablename__ = "users"
    items: Mapped[list["Item"]] = relationship("Item", back_populates="owner")

class Item(Base, TimestampMixin):
    __tablename__ = "items"
    owner: Mapped["User"] = relationship("User", back_populates="items")
```

```python
# 正確：在查詢時明確指定 eager loading
from sqlalchemy.orm import selectinload

stmt = select(User).options(selectinload(User.items)).where(User.id == user_id)
result = await session.execute(stmt)
user = result.scalar_one()
# user.items 已載入，可安全存取

# 錯誤：查詢後直接存取 relationship（async 模式會噴 MissingGreenlet）
stmt = select(User).where(User.id == user_id)
result = await session.execute(stmt)
user = result.scalar_one()
print(user.items)  # MissingGreenlet error!
```

## Migration (Alembic)

- 每個 schema 變更都要產生 migration：`make generate_migration`
- 新增 model 後必須在 `alembic/env.py` 中 import model
- Migration 檔的 `upgrade()` 和 `downgrade()` 都要實作
- NEVER 手動修改已 apply 的 migration 檔
- NEVER 在 migration 中寫業務邏輯或複雜的 data migration（用獨立腳本）

## 連線池最佳實踐

以下設定可避免連線池耗盡與服務啟動 hang 住：

### connect_timeout 必設

DSN 中必須包含 `connect_timeout`，防止 DB 連線 hang 時無限等待：

```python
# config.py — 確保 DSN 包含 connect_timeout
DATABASE_URL = "postgresql://user:pass@host:5432/db?connect_timeout=10"
```

若 DSN 來自環境變數且不一定包含 connect_timeout，在 engine 建立時設定：

```python
engine = create_async_engine(
    settings.async_database_url,
    pool_size=settings.DB_POOL_SIZE,
    max_overflow=settings.DB_MAX_OVERFLOW,
    pool_recycle=settings.DB_POOL_RECYCLE,
    pool_pre_ping=True,              # 連線前自動 ping，偵測失效連線
    connect_args={"timeout": 10},    # asyncpg connect timeout
)
```

### Pool Sizing 原則

- `pool_size`：常駐連線數，設為該服務**正常併發**的 DB 查詢數
- `max_overflow`：突發流量時可額外建立的連線數
- 所有服務的 `pool_size + max_overflow` 總和**不得超過** PostgreSQL `max_connections`
- 多服務架構下，每個服務的 pool 不要太大（建議 pool_size=5~20）

### 多服務同時重啟場景

多服務同時重啟時，各服務同時嘗試建立最小連線數 → DB max_connections 耗盡。解法：

- `pool_size` 不要設太高（讓連線按需建立）
- DSN 必須有 `connect_timeout`，確保連線失敗時快速 fail 而非 hang

## Transaction 規範

### 禁止在 Transaction 內呼叫外部服務

DB transaction 持有連線和鎖，外部呼叫（HTTP、Redis、第三方 API）的回應時間不可控。
Transaction 內呼叫外部服務 → 連線長時間被佔用 → 連線池耗盡。

```python
# 錯誤：顯式 transaction 內呼叫外部服務
async def create_order(self, data: OrderCreate, user_id: int) -> Order:
    async with self.session.begin():
        order = Order(**data.model_dump(), user_id=user_id)
        self.session.add(order)
        await self.session.flush()
        await self.notification_client.send(user_id, "Order created")  # 禁止！
        return order
```

### 注意 `get_db` 的 commit 時機

本專案的 `get_db` 使用 `yield` pattern，commit 發生在 **endpoint 回傳之後**（dependency cleanup 階段）：

```python
async def get_db():
    async with async_session_factory() as session:
        try:
            yield session
            await session.commit()    # ← endpoint 回傳後才 commit
        except Exception:
            await session.rollback()
            raise
```

因此在 service 層中，`repo.create()` 只是 `flush`（寫入 DB 但未 commit）。如果 service 中呼叫外部服務，此時 transaction 仍未結束：

```python
# 注意：notification 在 commit 前就送出了
# 如果後續 commit 失敗，通知已發但資料未寫入
async def create_order(self, data: OrderCreate, user_id: int) -> Order:
    order = await self.repo.create(order)   # flush，但尚未 commit
    await self.notification_client.send(...) # commit 尚未發生！
    return order
    # ← endpoint 回傳後，get_db 才 commit
```

**需要確保外部呼叫在 commit 之後執行的場景**（如通知、webhook），使用 BackgroundTasks：

```python
# 正確：用 BackgroundTasks 確保 commit 完成後才執行
@router.post("")
async def create_order(
    data: OrderCreate,
    background_tasks: BackgroundTasks,
    current_user: User = Depends(get_current_active_user),
    service: OrderService = Depends(get_order_service),
) -> DataResponse[OrderResponse]:
    order = await service.create_order(data, current_user.id)
    background_tasks.add_task(send_notification, current_user.id, order.id)
    return DataResponse(data=OrderResponse.model_validate(order))
```

### 同理適用於 Redis 操作

Cache invalidation 理想上應在 commit 之後執行。但因 `get_db` 的 yield pattern，service 層無法得知 commit 是否成功。務實做法：在 service 中 invalidate 快取是可接受的（最壞情況只是下次多一次 cache miss），但不要在 flush 之前就 invalidate。

## 禁止事項

- **NEVER** 使用 raw SQL string，一律用 SQLAlchemy ORM 或 `text()` 搭配參數綁定
- **NEVER** 在 model 中寫業務邏輯（業務邏輯屬於 service 層）
- **NEVER** 使用 sync SQLAlchemy（`Session`），只用 async（`AsyncSession`）
- **NEVER** 在 model 中直接 import service 或 router（避免循環依賴）
- **NEVER** 使用 `Float` 存金額
- **NEVER** 存不帶 timezone 的時間
- **NEVER** 在 JSONB 中塞可以正規化的關聯資料
- **NEVER** 使用 PG `Enum()` 型別（用 `String` + Pydantic Enum 驗證替代）
- **NEVER** 用 `String` 存狀態值而沒有對應的 Pydantic Enum 驗證
- **NEVER** 在 DB transaction 內呼叫外部服務（HTTP、Redis、第三方 API）
