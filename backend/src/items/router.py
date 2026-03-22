from fastapi import APIRouter, BackgroundTasks, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from src.auth.dependencies import get_current_active_user
from src.auth.models import User
from src.core.database import get_db
from src.core.dependencies import PaginationParams
from src.core.logging import logger
from src.core.schemas.base import DataResponse, DetailResponse, PaginatedResponse
from src.items.repository import ItemRepository
from src.items.schemas import ItemCreate, ItemResponse, ItemUpdate
from src.items.service import ItemService

router = APIRouter(prefix="/items", tags=["items"])


def get_item_service(db: AsyncSession = Depends(get_db)) -> ItemService:
    return ItemService(ItemRepository(db))


def _log_item_created(item_id: int, owner_id: int) -> None:
    logger.info("item_created", item_id=item_id, owner_id=owner_id)


@router.post("")
async def create_item(
    data: ItemCreate,
    background_tasks: BackgroundTasks,
    current_user: User = Depends(get_current_active_user),
    service: ItemService = Depends(get_item_service),
) -> DataResponse[ItemResponse]:
    item = await service.create_item(data, current_user.id)
    background_tasks.add_task(_log_item_created, item.id, current_user.id)
    return DataResponse(data=ItemResponse.model_validate(item))


@router.get("")
async def list_my_items(
    pagination: PaginationParams = Depends(),
    current_user: User = Depends(get_current_active_user),
    service: ItemService = Depends(get_item_service),
) -> PaginatedResponse[ItemResponse]:
    items, total, pages = await service.get_my_items(
        current_user.id, page=pagination.page, size=pagination.size
    )
    return PaginatedResponse(
        data=[ItemResponse.model_validate(i) for i in items],
        total=total,
        page=pagination.page,
        size=pagination.size,
        pages=pages,
    )


@router.get("/{item_id}")
async def get_item(
    item_id: int,
    current_user: User = Depends(get_current_active_user),
    service: ItemService = Depends(get_item_service),
) -> DataResponse[ItemResponse]:
    item = await service.get_item(item_id)
    return DataResponse(data=ItemResponse.model_validate(item))


@router.patch("/{item_id}")
async def update_item(
    item_id: int,
    data: ItemUpdate,
    current_user: User = Depends(get_current_active_user),
    service: ItemService = Depends(get_item_service),
) -> DataResponse[ItemResponse]:
    item = await service.update_item(item_id, data, current_user.id)
    return DataResponse(data=ItemResponse.model_validate(item))


@router.delete("/{item_id}")
async def delete_item(
    item_id: int,
    current_user: User = Depends(get_current_active_user),
    service: ItemService = Depends(get_item_service),
) -> DetailResponse:
    await service.delete_item(item_id, current_user.id)
    return DetailResponse(detail="Item deleted")
