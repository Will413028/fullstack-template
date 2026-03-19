from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from src.auth.models import User
from src.core.repository.base import BaseRepository


class UserRepository(BaseRepository[User]):
    def __init__(self, session: AsyncSession):
        super().__init__(User, session)

    async def get_by_account(self, account: str) -> User | None:
        stmt = select(User).where(User.account == account)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()
