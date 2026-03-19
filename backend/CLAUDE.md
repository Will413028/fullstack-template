# FastAPI Template

Production-ready FastAPI template with Pragmatic DDD architecture, async SQLAlchemy, and JWT authentication.

## Tech Stack

- **Runtime:** Python 3.12, FastAPI, Uvicorn
- **ORM:** SQLAlchemy 2.0 (async) + asyncpg
- **Auth:** JWT (python-jose) + bcrypt (passlib)
- **Migrations:** Alembic (async)
- **Logging:** structlog
- **Package Manager:** uv

## Quick Reference

```bash
make run                 # Start dev server
make lint                # Format + lint (ruff)
make test                # Run pytest
make migration           # Apply migrations
make generate_migration  # Auto-generate migration
```

## Architecture: Pragmatic DDD

Each module is a flat directory with clearly separated responsibilities:

| File | DDD Layer | Responsibility |
|------|-----------|---------------|
| `models.py` | Domain | SQLAlchemy models (domain entities) |
| `schemas.py` | Presentation | Pydantic DTOs (request/response) |
| `repository.py` | Infrastructure | Data access (extends BaseRepository) |
| `service.py` | Application | Business logic (raises AppException, NEVER HTTPException) |
| `router.py` | Presentation | API endpoints (NO try/except, global handler catches all) |
| `dependencies.py` | - | FastAPI Depends for the module |

## Project Structure

```
src/
├── main.py                      # create_app() factory
├── core/
│   ├── config.py                # Pydantic Settings
│   ├── database.py              # AsyncSession, get_db
│   ├── logging.py               # structlog config
│   ├── security.py              # OAuth2, JWT, password hashing
│   ├── exceptions.py            # AppException family
│   ├── exception_handlers.py    # Global exception → JSON response
│   ├── middleware.py            # ProcessTime, RequestId
│   ├── dependencies.py         # PaginationParams
│   ├── models/base.py          # Base, TimestampMixin, SoftDeleteMixin
│   ├── schemas/base.py         # DataResponse[T], PaginatedResponse[T]
│   └── repository/base.py      # BaseRepository[T] async generic CRUD
├── auth/                        # Authentication module
├── items/                       # CRUD example module
└── health/                      # Health check
```

## Key Conventions

1. **Service layer NEVER raises HTTPException** — only `AppException` subclasses from `core/exceptions.py`
2. **Router has NO try/except** — global exception handler in `core/exception_handlers.py` handles everything
3. **All DB operations are async** — use `AsyncSession`, `await session.execute()`
4. **Repository pattern** — extend `BaseRepository[T]` for CRUD, add custom queries as methods
5. **Dependency injection** — create `get_<module>_service()` function that wires repo → service
6. **Pydantic schemas use `from_attributes=True`** for ORM model conversion

## Error Handling Pattern

```python
# core/exceptions.py — define domain exceptions
class NotFoundException(AppException):
    def __init__(self, detail="Resource not found"):
        super().__init__(detail=detail, status_code=404)

# service.py — raise domain exceptions
async def get_item(self, item_id: int) -> Item:
    item = await self.repo.get_by_id(item_id)
    if not item:
        raise NotFoundException(detail=f"Item {item_id} not found")
    return item

# router.py — NO try/except needed
@router.get("/{item_id}")
async def get_item(item_id: int, service=Depends(get_item_service)):
    item = await service.get_item(item_id)
    return DataResponse(data=ItemResponse.model_validate(item))
```

## Adding a New Module

1. Create directory `src/<module>/` with: `__init__.py`, `models.py`, `schemas.py`, `repository.py`, `service.py`, `router.py`
2. Model extends `Base` + `TimestampMixin` from `core/models/base.py`
3. Repository extends `BaseRepository[YourModel]`
4. Service takes repository in `__init__`, raises `AppException` subclasses
5. Router uses `Depends()` to inject service
6. Register router in `src/main.py` → `create_app()`
7. Import model in `alembic/env.py`
8. Run `make generate_migration` then `make migration`

Or use: `/add-module <name>` to auto-generate the skeleton.
