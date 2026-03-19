# Fullstack Template 建置計畫

## 背景

我在 `/Users/will/github/fastapi-template` 已經完成了 FastAPI backend 的重構（Pragmatic DDD 架構）。
現在要把它搬進這個 monorepo 的 `backend/` 目錄，並加上 Next.js 前端。

## 已完成的 FastAPI Backend

在 `fastapi-template` repo 中已完成：

### 架構：Pragmatic DDD + Async SQLAlchemy

```
src/
├── main.py                      # create_app() factory
├── core/
│   ├── config.py                # Pydantic Settings
│   ├── database.py              # AsyncSession + asyncpg
│   ├── logging.py               # structlog
│   ├── security.py              # OAuth2, JWT, bcrypt
│   ├── exceptions.py            # AppException 家族
│   ├── exception_handlers.py    # 全域 exception → JSON
│   ├── middleware.py            # ProcessTime, RequestId
│   ├── dependencies.py         # PaginationParams
│   ├── models/base.py          # Base, TimestampMixin, SoftDeleteMixin
│   ├── schemas/base.py         # DataResponse[T], PaginatedResponse[T]
│   └── repository/base.py      # BaseRepository[T] async generic CRUD
├── auth/                        # JWT 認證 (register, login, me)
│   ├── models.py / schemas.py / repository.py / service.py / router.py
│   ├── dependencies.py         # get_current_user, RoleRequired
│   └── constants.py            # Role enum
├── items/                       # CRUD 範例模組 (5 endpoints + 分頁 + ownership)
│   ├── models.py / schemas.py / repository.py / service.py / router.py
└── health/
    └── router.py               # GET /health, GET /health/ready
```

### 關鍵設計決策
- **Async SQLAlchemy** — asyncpg + AsyncSession
- **Service 層只拋 AppException**，不拋 HTTPException
- **Router 無 try/except** — 靠全域 exception handler
- **BaseRepository[T]** 泛型 async CRUD
- **DATABASE_URL** 自動轉換 postgresql:// → postgresql+asyncpg://

### 技術棧
- Python 3.12, FastAPI, SQLAlchemy 2.0 (async), asyncpg
- JWT (python-jose) + bcrypt (passlib)
- Alembic (async), structlog, Ruff, uv, pytest-asyncio

### Claude Code 設定（也已完成）
- `CLAUDE.md` — 架構說明 + conventions
- `.claude/skills/add-module.md` — `/add-module <name>` 建模組骨架
- `.claude/skills/add-crud.md` — `/add-crud <entity> --fields "..."` 產生完整 CRUD

---

## 待辦事項

### Step 1：搬移 Backend

把 `/Users/will/github/fastapi-template` 的內容搬進 `backend/`：

```
fullstack-template/
├── backend/              # FastAPI（從 fastapi-template 搬入）
│   ├── src/
│   ├── tests/
│   ├── alembic/
│   ├── alembic.ini
│   ├── pyproject.toml
│   ├── Makefile
│   ├── Dockerfile
│   ├── .env-example
│   └── ...
├── frontend/             # Next.js（待建立）
├── docker-compose.yml    # 串接前後端 + DB
├── CLAUDE.md             # 統一規範（需重寫，涵蓋前後端）
├── .claude/skills/       # 前後端都能用的 skills
└── README.md
```

注意搬移後需要調整：
- `backend/Makefile` 路徑
- `backend/Dockerfile` 的 WORKDIR
- `backend/alembic.ini` 的 script_location

### Step 2：建立 Next.js Frontend

（基本架構，之後再細化）

### Step 3：docker-compose.yml

- PostgreSQL service
- Backend service (FastAPI)
- Frontend service (Next.js)

### Step 4：更新 CLAUDE.md

根目錄的 CLAUDE.md 涵蓋前後端的規範。

---

## 使用者偏好

- 使用繁體中文溝通
- 使用 async SQLAlchemy（不用 sync）
- 接案用的 production-ready template
- 搭配 Claude Code 輔助開發
