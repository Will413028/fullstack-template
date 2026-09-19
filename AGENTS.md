# Fullstack Template

Production-ready fullstack monorepo with FastAPI backend, Next.js frontend, and Docker Compose deployment.

## Development Workflow（必讀）

1. 需求或架構取捨未定時，先釐清設計；需求明確、低風險的既有行為修改，直接實作並驗證受影響範圍。
2. 沿用既有 roadmap／Pending 作主清單，複雜或跨模組項目才附實作與驗證計畫；複雜或高風險改動完成後做 review。
3. Skills 依具體任務需要選用；bug fix 優先先驗紅燈再修到綠燈，不依賴特定 plugin。

**依行為與風險安排驗證：**
- 新功能與 bugfix 用測試覆蓋可觀察行為、失敗條件與相關回歸；既有測試足以覆蓋時，不為每個 module / function 機械補測試。
- 純文件或格式修改做相關靜態檢查，不要求新增行為測試。
- Backend 測試用 `pytest`，Frontend 測試用 `vitest`
- 測試必須通過 CI（`make test` / `pnpm test`）才能視為 task completed
- Coverage target: backend 80%+, frontend 合理覆蓋關鍵路徑

## Monorepo Structure

```
├── backend/                # FastAPI (Pragmatic DDD, async SQLAlchemy)
├── frontend/               # Next.js 16 (App Router, TypeScript, Tailwind, shadcn/ui)
├── docker-compose.yml      # Production stack (postgres + backend + frontend)
├── docker-compose.dev.yml  # Dev overrides (hot reload, volume mounts)
├── Makefile                # Docker Compose shortcuts
└── .env-example            # Environment variable template
```

## Quick Reference

```bash
# Docker Compose (from root)
make dev              # Start dev stack with hot reload
make up               # Start production stack (detached)
make down             # Stop stack
make down-v           # Stop stack and remove volumes
make build            # Build Docker images
make status           # Show running containers
make logs             # Tail all logs
make logs-backend     # Tail backend logs
make logs-frontend    # Tail frontend logs
make restart          # Restart all services
make migration        # Run Alembic migrations
make seed             # Create admin user

# Backend development (from backend/)
make run              # Start FastAPI dev server
make test             # Run pytest
make lint             # Ruff format + check

# Frontend development (from frontend/)
pnpm dev              # Start Next.js dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript check + Biome lint
pnpm format           # Biome format
```

## Architecture

### Backend — Pragmatic DDD

See `backend/AGENTS.md` for full details. Key patterns:
- **Service layer NEVER raises HTTPException** — only `AppException` subclasses
- **Router has NO try/except** — global exception handler catches all
- **BaseRepository[T]** — generic async CRUD
- **All DB operations are async** (AsyncSession + asyncpg)

### Frontend — Feature-based Next.js

- **Next.js 16** with App Router, React 19, TypeScript
- **pnpm** package manager
- **Tailwind CSS v4** + **shadcn/ui** components
- **Biome** for linting/formatting (not ESLint)
- **Tanstack Query** for server state
- **next-intl** for i18n (en, zh-TW)
- **React Hook Form** + **Zod** for form validation

Frontend structure:
```
src/
├── app/[locale]/         # File-based routing with i18n
│   ├── (auth)/           # Auth pages (login)
│   ├── (dashboard)/      # Protected pages
│   └── (marketing)/      # Public pages
├── components/
│   ├── ui/               # shadcn/ui components
│   └── layout/           # Header, footer, dashboard-shell
├── features/             # Feature modules (auth, etc.)
├── hooks/                # Custom React hooks
├── lib/                  # Utilities (api-client, server-auth, utils)
├── providers/            # React Query provider
├── store/                # Zustand client UI state
├── i18n/                 # Internationalization config
└── proxy.ts              # Presence redirect + CSP (advisory)
```

### Docker Compose Deployment

Two compose files for different environments:
- **`docker-compose.yml`** — production mode (built images)
- **`docker-compose.dev.yml`** — dev overrides (volume mounts, hot reload)

Services: postgres → migrate (alembic) → backend (:8000) → frontend (:3000)

## Design Rules

開發新功能前，必須先閱讀對應的設計規則。這些規則是強制性的，所有程式碼必須遵守：

- **Database 設計**：`docs/rules/database-design.md` — 命名規範、Model 結構、型別選擇、Migration
- **API 設計**：`docs/rules/api-design.md` — URL 設計、Response 格式、Schema/Router/Service 規範

## Conventions

- Communicate in 繁體中文
- Use async SQLAlchemy (never sync) on backend
- Use Biome (not ESLint/Prettier) on frontend
- Use pnpm (not npm/yarn) on frontend
- Use uv (not pip) on backend
