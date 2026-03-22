# Redis Design Rules

本專案 Redis 用於快取、session 儲存、和 rate limiting。以下規則確保 key 設計一致且可維護。

## 何時該用快取

快取增加系統複雜度（invalidation、一致性問題），不是所有查詢都需要快取：

- **先量測，再快取** — 帶 index 的簡單 DB 查詢通常夠快（< 10ms），不需要快取
- **適合快取的場景**：讀多寫少的資料、計算成本高的聚合查詢、外部 API 結果
- **不適合快取的場景**：寫入頻繁（快取不斷被 invalidate）、資料一致性要求極高、單一使用者才會存取的冷資料
- NEVER 在沒有效能問題的情況下預先加快取

## 連線設計

### 使用 redis.asyncio

與專案的 async 架構一致，Redis client 必須使用 async 版本：

```python
from redis.asyncio import Redis

redis_client = Redis.from_url(settings.REDIS_URL, decode_responses=True)
```

### 連線管理

- Redis client 在 `core/redis.py` 中初始化，作為 singleton
- 透過 FastAPI dependency 注入，不要在 service 中直接 import
- App shutdown 時必須呼叫 `await redis_client.aclose()`

```python
# core/redis.py
from redis.asyncio import Redis
from src.core.config import settings

redis_client = Redis.from_url(settings.REDIS_URL, decode_responses=True)

async def get_redis() -> Redis:
    return redis_client
```

```python
# main.py — lifespan 中處理
@asynccontextmanager
async def lifespan(app: FastAPI):
    yield
    await redis_client.aclose()
```

## Key 命名規範

### 格式

使用冒號 `:` 分隔的階層結構：

```
{app}:{domain}:{identifier}:{sub-key}
```

### 規則

| 規則 | 正確 | 錯誤 |
|------|------|------|
| 使用冒號分隔 | `app:user:123:profile` | `app_user_123_profile` |
| domain 用 singular | `app:user:123` | `app:users:123` |
| ID 直接放在 domain 後 | `app:item:456` | `app:item:id:456` |
| 不含空白或特殊字元 | `app:order-item:789` | `app:order item:789` |

### 常見 Key Pattern

```python
# 快取單一資源
f"app:user:{user_id}:profile"

# 快取列表（用 query 參數的 hash 區分不同查詢條件）
f"app:item:list:{owner_id}:{query_hash}"   # query_hash = MD5(query_params) 前 8 碼

# Session
f"app:session:{session_id}"

# Rate limit
f"app:ratelimit:{client_ip}:{endpoint}"

# 分散式鎖（短 TTL 防止死鎖）
f"app:lock:{resource_type}:{resource_id}"

# 計數器/流水號（INCR，每日自動過期）
f"app:seq:{prefix}:{date}"                 # e.g., app:seq:order:20260322

# 一次性 Token（用完即刪，TTL 作為兜底）
f"app:token:{purpose}:{token_id}"          # e.g., app:token:checkout:{uuid}
```

### 列表快取的 queryHash

當列表查詢有多種篩選組合時，用查詢參數的 hash 值作為 key 的一部分：

```python
import hashlib

def make_list_cache_key(prefix: str, owner_id: int, **query_params) -> str:
    """產生列表快取的 key，query_params 的 hash 確保不同查詢不會互相覆蓋。"""
    sorted_params = sorted(query_params.items())
    param_str = "&".join(f"{k}={v}" for k, v in sorted_params)
    query_hash = hashlib.md5(param_str.encode()).hexdigest()[:8]
    return f"app:{prefix}:list:{owner_id}:{query_hash}"
```

## TTL 策略

### 必須設定 TTL

所有 key 都**必須**設定 TTL，禁止永久 key（除非有明確理由並加註解）。

### 建議 TTL 值

| 用途 | TTL | 說明 |
|------|-----|------|
| 熱點資料快取 | 5-15 分鐘 | 頻繁讀取、變化少 |
| 列表/分頁快取 | 1-5 分鐘 | 資料變化較頻繁 |
| Session | 與 token 同步 | access token 30 分鐘 |
| Rate limit window | 依窗口大小 | 例如 60 秒 |
| 分散式鎖 | 30 秒 - 5 分鐘 | 防止死鎖 |

```python
# 正確：設定 TTL（一行搞定，原子操作）
await redis.set(key, value, ex=300)  # 5 分鐘

# 錯誤：沒有 TTL
await redis.set(key, value)  # 永遠不會過期！

# 錯誤：分開 set + expire（不必要的複雜度，且非 pipeline 時有短暫無 TTL 的窗口）
await redis.set(key, value)
await redis.expire(key, 300)
```

Pipeline 保留給真正需要批次操作的場景（如同時讀寫多個 key），不用於單一 key 的 set + expire。

## 資料結構選擇

| 場景 | 結構 | 範例 |
|------|------|------|
| 單一值快取 | `String` (SET/GET) | 快取 JSON serialized object |
| 物件屬性 | `Hash` (HSET/HGET) | 使用者 profile 的各欄位 |
| 排行榜/排序 | `Sorted Set` (ZADD/ZRANGE) | 熱門文章排名 |
| 佇列 | `List` (LPUSH/BRPOP) | 簡易任務佇列 |
| 唯一集合 | `Set` (SADD/SISMEMBER) | 線上使用者列表 |
| 計數器 | `String` (INCR) | Rate limit 計數、流水號 |

### 進階結構範例

**Hash — 購物車 / 多欄位物件**

適合 field 會個別增刪的場景（如購物車的各品項）：

```python
cart_key = f"app:cart:{user_id}"

# 加入品項（field = item_id, value = quantity）
await redis.hset(cart_key, str(item_id), json.dumps({"qty": 2, "price": 100}))
await redis.expire(cart_key, 7 * 86400)  # 7 天

# 取得整個購物車
cart = await redis.hgetall(cart_key)

# 刪除單一品項
await redis.hdel(cart_key, str(item_id))
```

**INCR — 流水號 / 計數器**

原子操作，天然無 race condition：

```python
# 每日訂單流水號
seq_key = f"app:seq:order:{date.today().strftime('%Y%m%d')}"
seq_num = await redis.incr(seq_key)
if seq_num == 1:
    await redis.expire(seq_key, 86400)  # 首次建立時設 TTL
order_no = f"ORD{date.today().strftime('%Y%m%d')}{seq_num:06d}"
```

**一次性 Token**

用於確保操作只執行一次（如結帳確認）：

```python
# 產生 token（Preview 階段）
token_key = f"app:token:checkout:{checkout_id}"
await redis.set(token_key, json.dumps(checkout_data), ex=1800)  # 30 分鐘

# 驗證 + 消費 token（Confirm 階段）
data = await redis.getdel(token_key)  # 取值同時刪除，確保只用一次
if not data:
    raise AppException(detail="Checkout expired or already confirmed", status_code=400)
```

### JSON 序列化

快取 Python 物件時統一用 JSON：

```python
import json

# 寫入
await redis.set(key, json.dumps(data, default=str), ex=300)

# 讀取
cached = await redis.get(key)
if cached:
    return json.loads(cached)
```

- 用 `default=str` 處理 datetime 等不可序列化型別
- NEVER 使用 pickle 序列化（安全風險）

## Cache Pattern

### Cache-Aside（首選）

```python
class ItemService:
    def __init__(self, repo: ItemRepository, redis: Redis):
        self.repo = repo
        self.redis = redis

    async def get_item(self, item_id: int) -> ItemResponse:
        # 1. 查快取
        cache_key = f"app:item:{item_id}"
        cached = await self.redis.get(cache_key)
        if cached:
            return ItemResponse(**json.loads(cached))

        # 2. 查 DB
        item = await self.repo.get_by_id(item_id)
        if not item:
            raise NotFoundException(detail=f"Item {item_id} not found")

        # 3. 轉成 Pydantic schema 後寫快取
        response = ItemResponse.model_validate(item)
        await self.redis.set(
            cache_key,
            json.dumps(response.model_dump(), default=str),
            ex=300,
        )
        return response
```

快取中儲存的是 Pydantic schema（可序列化），不是 ORM model。從快取讀取時直接還原成 schema 回傳。

### Cache Invalidation

- 資料寫入（create/update/delete）後，**立即刪除**相關快取
- 使用 `delete()` 而非設定新值（讓下次讀取重新填充）
- 列表快取在任何單筆資料異動後都要清除
- 因 `get_db` 的 yield pattern，service 層無法得知 commit 是否成功。務實做法：在 service 中 invalidate 是可接受的（最壞情況只是多一次 cache miss），但不要在 flush 之前就 invalidate。需要嚴格保證時，用 `BackgroundTasks`（見 `docs/rules/database-design.md` Transaction 規範）

```python
async def update_item(self, item_id: int, data: ItemUpdate, owner_id: int) -> ItemResponse:
    item = await self.repo.get_by_id(item_id)
    if not item:
        raise NotFoundException(detail=f"Item {item_id} not found")
    self._check_ownership(item, owner_id)
    updated = await self.repo.update(item, data.model_dump(exclude_unset=True))

    # Invalidate 單筆快取
    await self.redis.delete(f"app:item:{item_id}")

    # Invalidate 列表快取（delete 不支援 wildcard，必須用 SCAN 找出再刪除）
    await self._delete_by_pattern(f"app:item:list:{owner_id}:*")

    return ItemResponse.model_validate(updated)

async def _delete_by_pattern(self, pattern: str) -> int:
    """用 SCAN 收集所有符合 pattern 的 key，一次批次刪除。NEVER 用 KEYS 命令。"""
    keys = [key async for key in self.redis.scan_iter(match=pattern, count=100)]
    if keys:
        return await self.redis.delete(*keys)
    return 0
```

## 除錯指引

### 何時懷疑快取問題

遇到以下現象時，應**優先**考慮 Redis 快取：

1. **DB 已更新但 API 回傳舊資料** — 最常見的快取問題
2. **直接改 DB 後行為不符預期** — DB 修改不會觸發快取清除
3. **間歇性資料不一致** — 部分請求 cache hit（舊資料），部分 cache miss（新資料）
4. **重啟服務後問題消失** — 可能是快取 TTL 還沒過期

### 除錯步驟

```bash
# 1. 檢查特定 key 是否存在
redis-cli GET "app:item:123"

# 2. 查看 TTL
redis-cli TTL "app:item:123"

# 3. 刪除特定 key（安全操作，最壞情況只是多一次 DB 讀取）
redis-cli DEL "app:item:123"

# 4. 掃描某 pattern 的所有 key（不用 KEYS，用 SCAN）
redis-cli --scan --pattern "app:item:*"
```

### 清除快取是安全操作

刪除快取的最壞後果只是下次查詢多一次 DB 讀取（cache miss）。當不確定時，大膽刪除。

## 禁止事項

- **NEVER** 使用永久 key（沒有 TTL）
- **NEVER** 使用 pickle 序列化
- **NEVER** 在 key 中放入敏感資訊（密碼、token 值）
- **NEVER** 使用 `KEYS *` 命令（阻塞操作，用 `SCAN` 替代）
- **NEVER** 在 Redis 中存超大 value（單一 value 不超過 1 MB）
- **NEVER** 使用 sync redis client（與專案 async 架構不一致）
- **NEVER** 直接在 router 或 repository 層操作 Redis（快取邏輯屬於 service 層）
