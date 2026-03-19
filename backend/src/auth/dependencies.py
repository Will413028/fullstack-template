from typing import Annotated

from fastapi import Depends
from jose import JWTError, jwt
from sqlalchemy.ext.asyncio import AsyncSession

from src.auth.constants import Role
from src.auth.models import User
from src.auth.repository import UserRepository
from src.auth.service import AuthService
from src.core.config import settings
from src.core.database import get_db
from src.core.exceptions import ForbiddenException, UnauthorizedException
from src.core.security import oauth2_scheme


def get_auth_service(db: AsyncSession = Depends(get_db)) -> AuthService:
    return AuthService(UserRepository(db))


async def get_current_user(
    token: Annotated[str, Depends(oauth2_scheme)],
    service: AuthService = Depends(get_auth_service),
) -> User:
    try:
        payload = jwt.decode(token, settings.SECRET_KEY, algorithms=[settings.ALGORITHM])
        account: str | None = payload.get("sub")
        if account is None:
            raise UnauthorizedException(detail="Could not validate credentials")
    except JWTError as e:
        raise UnauthorizedException(detail="Could not validate credentials") from e

    return await service.get_user_by_account(account)


async def get_current_active_user(
    current_user: Annotated[User, Depends(get_current_user)],
) -> User:
    if current_user.is_disabled:
        raise UnauthorizedException(detail="User account is disabled")
    return current_user


class RoleRequired:
    def __init__(self, *roles: Role):
        self.roles = roles

    async def __call__(
        self, current_user: Annotated[User, Depends(get_current_active_user)]
    ) -> User:
        if current_user.role not in [r.value for r in self.roles]:
            raise ForbiddenException(detail="Insufficient permissions")
        return current_user
