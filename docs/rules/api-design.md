# API Design Rules

本專案的 API 遵循 Pragmatic DDD 架構，分層為 Router → Service → Repository。

## URL 設計

### 路徑命名

- 使用 **複數名詞 kebab-case**：`/items`, `/order-items`, `/payment-records`
- 資源 ID 用路徑參數：`/items/{item_id}`
- 巢狀資源最多兩層：`/users/{user_id}/items`
- NEVER 在 URL 中使用動詞：`/items/create` → 用 `POST /items`
- NEVER 用角色作為路徑切分：`/buyer/orders` → 用 `/orders` 搭配權限控制

### Action Endpoint（非 CRUD 操作）

不屬於標準 CRUD 的業務動作，使用 `POST /{resource}/{id}/{action}` 格式：

```
POST /orders/{order_id}/cancel      # 取消訂單
POST /orders/{order_id}/confirm     # 確認訂單
POST /products/{product_id}/approve # 審核通過
POST /users/{user_id}/deactivate    # 停用帳號
```

- Action 一律用 `POST`，不用 `PATCH`
- Action 名稱用動詞原型：`cancel`, `approve`, `confirm`

### HTTP Method 語意

| 操作 | Method | URL | Return Type |
|------|--------|-----|-------------|
| 建立 | `POST` | `/items` | `-> DataResponse[ItemResponse]` |
| 列表 | `GET` | `/items` | `-> PaginatedResponse[ItemResponse]` |
| 取得 | `GET` | `/items/{item_id}` | `-> DataResponse[ItemResponse]` |
| 更新 | `PATCH` | `/items/{item_id}` | `-> DataResponse[ItemResponse]` |
| 取代 | `PUT` | `/items/{item_id}` | `-> DataResponse[ItemResponse]` |
| 刪除 | `DELETE` | `/items/{item_id}` | `-> DetailResponse` |

- 部分更新用 `PATCH`（搭配 `exclude_unset=True`），整體取代用 `PUT`
- 優先使用 `PATCH`，除非語意明確需要 `PUT`

## Response 格式

所有 response 必須使用 `core/schemas/base.py` 中定義的標準格式：

```python
# 單一資源
DataResponse[ItemResponse]     # {"data": {...}}

# 列表（不分頁）
ListDataResponse[ItemResponse] # {"data": [{...}, ...]}

# 分頁列表
PaginatedResponse[ItemResponse] # {"data": [...], "total": 100, "page": 1, "size": 20, "pages": 5}

# 操作結果（無資料回傳）
DetailResponse                  # {"detail": "Item deleted"}
```

- NEVER 直接回傳 model 或裸 dict
- NEVER 自定義 response wrapper（使用現有的 `DataResponse`, `PaginatedResponse` 等）

## Schema 設計 (Pydantic)

### 命名規範

每個資源至少要有三個 schema：

| Schema | 用途 | 範例 |
|--------|------|------|
| `{Name}Create` | 建立請求 | `ItemCreate` |
| `{Name}Update` | 更新請求（所有欄位 Optional） | `ItemUpdate` |
| `{Name}Response` | 回應 | `ItemResponse` |

### Response Schema 必須

```python
class ItemResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)  # 必須：允許從 ORM model 轉換

    id: int
    title: str
    description: str | None
    owner_id: int
    created_at: datetime.datetime
    updated_at: datetime.datetime
```

- `from_attributes=True` 是必要的，用於 `ItemResponse.model_validate(orm_obj)`
- 回應中包含 `id`, `created_at`, `updated_at`
- 敏感欄位（password hash 等）NEVER 出現在 Response schema 中

### Update Schema

- 所有欄位都是 `Optional`
- 搭配 `model_dump(exclude_unset=True)` 只更新有傳入的欄位

```python
class ItemUpdate(BaseModel):
    title: str | None = None
    description: str | None = None
```

## Router 規範

### 結構

```python
router = APIRouter(prefix="/items", tags=["items"])


def get_item_service(db: AsyncSession = Depends(get_db)) -> ItemService:
    return ItemService(ItemRepository(db))
```

### 必須遵守

1. **NO try/except** — 所有錯誤由 global exception handler 處理
2. **使用 Depends()** 注入 service，不要在 endpoint 內建立
3. **使用 return type annotation** — 用 `-> DataResponse[ItemResponse]` 標注回傳型別，FastAPI 自動推導 response_model，不需要另外設 `response_model=`
4. **認證用 Depends** — `current_user: User = Depends(get_current_active_user)`

### 分頁

使用 `PaginationParams` dependency：

```python
@router.get("")
async def list_items(
    pagination: PaginationParams = Depends(),
    service: ItemService = Depends(get_item_service),
) -> PaginatedResponse[ItemResponse]:
    items, total, pages = await service.get_items(page=pagination.page, size=pagination.size)
    return PaginatedResponse(
        data=[ItemResponse.model_validate(i) for i in items],
        total=total,
        page=pagination.page,
        size=pagination.size,
        pages=pages,
    )
```

## Service 規範

### 錯誤處理

- Service 只能 raise `AppException` 子類別（定義在 `core/exceptions.py`）
- **NEVER raise `HTTPException`** — 這是 FastAPI 的 presentation 層概念，不屬於 service 層
- 需要新的錯誤類型時，在 `core/exceptions.py` 中新增 `AppException` 子類別

```python
# 正確
raise NotFoundException(detail=f"Item {item_id} not found")
raise ForbiddenException(detail="Not the owner of this item")

# 錯誤
raise HTTPException(status_code=404, detail="Not found")
```

### 可用的 Exception 類別

| Exception | Status Code | 用途 |
|-----------|------------|------|
| `NotFoundException` | 404 | 資源不存在 |
| `AlreadyExistsException` | 409 | 資源已存在（重複） |
| `UnauthorizedException` | 401 | 未認證 |
| `ForbiddenException` | 403 | 無權限 |
| `AppException` | 自訂 | 其他業務錯誤 |

### 權限檢查

- 資源所有權檢查放在 service 層
- 用 `_check_ownership()` 之類的 private method 封裝

## Query 參數規範

- 參數命名用 **snake_case**：`created_after`, `sort_by`
- 時間格式一律 **ISO 8601 (UTC)**：`2026-03-22T14:30:00Z`
- NEVER 使用 Unix timestamp 作為 API 的時間參數
- 分頁參數：`page`（從 1 開始）、`size`（每頁筆數，上限 100）— 與 `PaginationParams` 一致
- 排序參數：`sort=created_at:desc` 或 `sort=-created_at`

```python
# Query 範例
GET /items?page=1&size=20&sort=created_at:desc&q=keyword&created_after=2026-01-01T00:00:00Z
```

## 版本與契約穩定性

API 上線後，同版本內必須維持向後相容：

- **可以**新增欄位（回應中多一個 field，前端可忽略）
- **NEVER** 刪除或重新命名已上線的欄位
- **NEVER** 改變欄位型別（如 `int` → `string`）
- 破壞性變更必須升版（`/v1/` → `/v2/`）

```python
# 正確：新增欄位（向後相容）
class ItemResponse(BaseModel):
    id: int
    title: str
    description: str | None
    category: str | None = None  # 新增欄位，有預設值

# 錯誤：刪除或改名已上線的欄位
class ItemResponse(BaseModel):
    id: int
    name: str           # 原本叫 title，改名了 → 破壞前端
```

## 前端 API 呼叫

前端使用 `apiClient`（`src/lib/api-client.ts`），提供 `get`, `post`, `put`, `patch`, `delete` 方法：

```typescript
// features/<name>/api/index.ts
import { apiClient } from "@/lib/api-client";

export async function getItems(page = 1, size = 20) {
  return apiClient.get<PaginatedResponse<Item>>("/items", {
    params: { page: String(page), size: String(size) },
  });
}

export async function createItem(data: ItemCreate) {
  return apiClient.post<DataResponse<Item>>("/items", data);
}

export async function updateItem(id: number, data: Partial<ItemCreate>) {
  return apiClient.patch<DataResponse<Item>>(`/items/${id}`, data);
}
```

- 所有 API 呼叫放在 `features/<name>/api/` 目錄
- 搭配 Tanstack Query hooks 使用（放在 `features/<name>/hooks/`）
- NEVER 在 component 中直接呼叫 `fetch()`

## 禁止事項

- **NEVER** 在 router 中寫 try/except
- **NEVER** 在 service 中 raise HTTPException
- **NEVER** 直接回傳 ORM model（必須轉成 Pydantic schema）
- **NEVER** 在 router 中寫業務邏輯（邏輯屬於 service 層）
- **NEVER** 在 endpoint 中直接操作 database session
- **NEVER** 自定義 response wrapper，使用 `DataResponse`, `PaginatedResponse`, `DetailResponse`
- **NEVER** 刪除或重新命名已上線的 response 欄位（向後相容）
- **NEVER** 使用 Unix timestamp，時間一律用 ISO 8601 UTC
