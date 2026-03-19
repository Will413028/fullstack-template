import math

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from src.core.repository.base import BaseRepository
from src.items.models import Item


class ItemRepository(BaseRepository[Item]):
    def __init__(self, session: AsyncSession):
        super().__init__(Item, session)

    async def get_by_owner_paginated(
        self, owner_id: int, *, page: int = 1, size: int = 20
    ) -> tuple[list[Item], int, int]:
        base = select(Item).where(Item.owner_id == owner_id)

        count_stmt = select(func.count()).select_from(base.subquery())
        total = (await self.session.execute(count_stmt)).scalar() or 0
        total_pages = math.ceil(total / size) if size > 0 else 0

        offset = (page - 1) * size
        stmt = base.offset(offset).limit(size)
        result = await self.session.execute(stmt)
        items = list(result.scalars().all())

        return items, total, total_pages
