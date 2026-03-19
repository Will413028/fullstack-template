---
name: add-module
description: Create a new module with all boilerplate files (models, schemas, repository, service, router)
user_invocable: true
---

# /add-module <name>

Create a complete new module following the project's Pragmatic DDD pattern.

## Instructions

Given the module name from the user's argument:

1. **Create `backend/src/<name>/`** directory with these files:

### `__init__.py`
Empty file.

### `models.py`
```python
from sqlalchemy import String
from sqlalchemy.orm import Mapped, mapped_column

from src.core.models.base import Base, TimestampMixin


class <Name>(Base, TimestampMixin):
    __tablename__ = "<name>s"

    id: Mapped[int] = mapped_column(primary_key=True, autoincrement=True)
    # TODO: Add your fields here
```

### `schemas.py`
```python
from pydantic import BaseModel, ConfigDict


class <Name>Create(BaseModel):
    pass  # TODO: Add create fields


class <Name>Update(BaseModel):
    pass  # TODO: Add update fields


class <Name>Response(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    # TODO: Add response fields
```

### `repository.py`
```python
from sqlalchemy.ext.asyncio import AsyncSession

from src.core.repository.base import BaseRepository
from src.<name>.models import <Name>


class <Name>Repository(BaseRepository[<Name>]):
    def __init__(self, session: AsyncSession):
        super().__init__(<Name>, session)
```

### `service.py`
```python
from src.core.exceptions import NotFoundException
from src.<name>.models import <Name>
from src.<name>.repository import <Name>Repository
from src.<name>.schemas import <Name>Create, <Name>Update


class <Name>Service:
    def __init__(self, repo: <Name>Repository):
        self.repo = repo

    async def create(self, data: <Name>Create) -> <Name>:
        obj = <Name>(**data.model_dump())
        return await self.repo.create(obj)

    async def get(self, id: int) -> <Name>:
        obj = await self.repo.get_by_id(id)
        if not obj:
            raise NotFoundException(detail=f"<Name> {id} not found")
        return obj

    async def update(self, id: int, data: <Name>Update) -> <Name>:
        obj = await self.get(id)
        return await self.repo.update(obj, data.model_dump(exclude_unset=True))

    async def delete(self, id: int) -> None:
        obj = await self.get(id)
        await self.repo.delete(obj)
```

### `router.py`
```python
from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from src.core.database import get_db
from src.core.dependencies import PaginationParams
from src.core.schemas.base import DataResponse, DetailResponse, PaginatedResponse
from src.<name>.repository import <Name>Repository
from src.<name>.schemas import <Name>Create, <Name>Response, <Name>Update
from src.<name>.service import <Name>Service

router = APIRouter(prefix="/<name>s", tags=["<name>s"])


def get_service(db: AsyncSession = Depends(get_db)) -> <Name>Service:
    return <Name>Service(<Name>Repository(db))


@router.post("", response_model=DataResponse[<Name>Response])
async def create(data: <Name>Create, service: <Name>Service = Depends(get_service)):
    obj = await service.create(data)
    return DataResponse(data=<Name>Response.model_validate(obj))


@router.get("", response_model=PaginatedResponse[<Name>Response])
async def list_all(
    pagination: PaginationParams = Depends(),
    service: <Name>Service = Depends(get_service),
):
    items, total, pages = await service.repo.get_paginated(page=pagination.page, size=pagination.size)
    return PaginatedResponse(
        data=[<Name>Response.model_validate(i) for i in items],
        total=total, page=pagination.page, size=pagination.size, pages=pages,
    )


@router.get("/{id}", response_model=DataResponse[<Name>Response])
async def get(id: int, service: <Name>Service = Depends(get_service)):
    obj = await service.get(id)
    return DataResponse(data=<Name>Response.model_validate(obj))


@router.patch("/{id}", response_model=DataResponse[<Name>Response])
async def update(id: int, data: <Name>Update, service: <Name>Service = Depends(get_service)):
    obj = await service.update(id, data)
    return DataResponse(data=<Name>Response.model_validate(obj))


@router.delete("/{id}", response_model=DetailResponse)
async def delete(id: int, service: <Name>Service = Depends(get_service)):
    await service.delete(id)
    return DetailResponse(detail="<Name> deleted")
```

2. **Register the router** in `backend/src/main.py`:
   - Add import: `from src.<name>.router import router as <name>_router`
   - Add to `create_app()`: `app.include_router(<name>_router)`
   - Add tag to `openapi_tags`: `{"name": "<name>s", "description": "<Name> management"}`

3. **Register model in Alembic** — add to `backend/alembic/env.py`:
   ```python
   from src.<name>.models import <Name>  # noqa: F401
   ```

4. **Create test stub** at `backend/tests/test_<name>.py`:
   ```python
   # TODO: Add tests for <name> module
   ```

5. **Remind the user:**
   - Fill in model fields in `models.py`
   - Fill in schema fields in `schemas.py`
   - Run `make generate_migration` to create the migration
   - Run `make migration` to apply it
