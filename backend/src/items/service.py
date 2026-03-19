from src.core.exceptions import ForbiddenException, NotFoundException
from src.items.models import Item
from src.items.repository import ItemRepository
from src.items.schemas import ItemCreate, ItemUpdate


class ItemService:
    def __init__(self, repo: ItemRepository):
        self.repo = repo

    async def create_item(self, data: ItemCreate, owner_id: int) -> Item:
        item = Item(title=data.title, description=data.description, owner_id=owner_id)
        return await self.repo.create(item)

    async def get_item(self, item_id: int) -> Item:
        item = await self.repo.get_by_id(item_id)
        if not item:
            raise NotFoundException(detail=f"Item {item_id} not found")
        return item

    async def get_my_items(
        self, owner_id: int, *, page: int = 1, size: int = 20
    ) -> tuple[list[Item], int, int]:
        return await self.repo.get_by_owner_paginated(owner_id, page=page, size=size)

    async def update_item(self, item_id: int, data: ItemUpdate, owner_id: int) -> Item:
        item = await self.get_item(item_id)
        self._check_ownership(item, owner_id)
        update_data = data.model_dump(exclude_unset=True)
        return await self.repo.update(item, update_data)

    async def delete_item(self, item_id: int, owner_id: int) -> None:
        item = await self.get_item(item_id)
        self._check_ownership(item, owner_id)
        await self.repo.delete(item)

    @staticmethod
    def _check_ownership(item: Item, owner_id: int) -> None:
        if item.owner_id != owner_id:
            raise ForbiddenException(detail="Not the owner of this item")
