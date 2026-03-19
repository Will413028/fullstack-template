import hashlib

from sqlalchemy import select, update
from sqlalchemy.ext.asyncio import AsyncSession

from src.auth.models import RefreshToken, User
from src.core.repository.base import BaseRepository


class UserRepository(BaseRepository[User]):
    def __init__(self, session: AsyncSession):
        super().__init__(User, session)

    async def get_by_account(self, account: str) -> User | None:
        stmt = select(User).where(User.account == account)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()


class RefreshTokenRepository:
    def __init__(self, session: AsyncSession):
        self.session = session

    async def create(self, token: RefreshToken) -> RefreshToken:
        self.session.add(token)
        await self.session.flush()
        await self.session.refresh(token)
        return token

    async def get_by_token(self, raw_token: str) -> RefreshToken | None:
        token_hash = hashlib.sha256(raw_token.encode()).hexdigest()
        stmt = select(RefreshToken).where(
            RefreshToken.token_hash == token_hash,
            RefreshToken.revoked.is_(False),
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def revoke(self, token: RefreshToken) -> None:
        token.revoked = True
        await self.session.flush()

    async def revoke_all_for_user(self, user_id: int) -> None:
        stmt = (
            update(RefreshToken)
            .where(RefreshToken.user_id == user_id, RefreshToken.revoked.is_(False))
            .values(revoked=True)
        )
        await self.session.execute(stmt)
