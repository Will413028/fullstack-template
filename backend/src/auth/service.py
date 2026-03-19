from datetime import timedelta

from src.auth.models import User
from src.auth.repository import UserRepository
from src.auth.schemas import UserCreateInput
from src.core.config import settings
from src.core.exceptions import (
    AlreadyExistsException,
    ForbiddenException,
    NotFoundException,
    UnauthorizedException,
)
from src.core.security import create_access_token, get_password_hash, verify_password


class AuthService:
    def __init__(self, repo: UserRepository):
        self.repo = repo

    async def register(self, data: UserCreateInput) -> str:
        existing = await self.repo.get_by_account(data.account)
        if existing:
            raise AlreadyExistsException(detail="User already exists")

        user = User(
            account=data.account,
            password=get_password_hash(data.password),
        )
        await self.repo.create(user)

        return create_access_token(
            data={"sub": user.account},
            expires_delta=timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES),
        )

    async def login(self, account: str, password: str) -> str:
        user = await self.repo.get_by_account(account)
        if not user:
            raise NotFoundException(detail="User not found")

        if not verify_password(password, user.password):
            raise ForbiddenException(detail="Invalid password or account")

        if user.is_disabled:
            raise UnauthorizedException(detail="User account is disabled")

        return create_access_token(
            data={"sub": user.account},
            expires_delta=timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES),
        )

    async def get_user_by_account(self, account: str) -> User:
        user = await self.repo.get_by_account(account)
        if not user:
            raise NotFoundException(detail="User not found")
        return user
