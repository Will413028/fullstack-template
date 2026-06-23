from typing import Annotated

import jwt
from fastapi import Depends, Request
from sqlalchemy.ext.asyncio import AsyncSession

from src.auth.cookies import ACCESS_COOKIE
from src.auth.models import User
from src.auth.repository import RefreshTokenRepository, UserRepository
from src.auth.service import AuthService
from src.core.config import settings
from src.core.database import get_db
from src.core.exceptions import UnauthorizedException
from src.core.security import oauth2_scheme


def get_auth_service(db: AsyncSession = Depends(get_db)) -> AuthService:
    return AuthService(UserRepository(db), RefreshTokenRepository(db))


async def get_current_user(
    request: Request,
    token: Annotated[str | None, Depends(oauth2_scheme)],
    service: AuthService = Depends(get_auth_service),
) -> User:
    # httpOnly cookie is the primary transport; fall back to the Authorization
    # header so Swagger and programmatic Bearer clients still work.
    access_token = request.cookies.get(ACCESS_COOKIE) or token
    if not access_token:
        raise UnauthorizedException(detail="Could not validate credentials")

    try:
        payload = jwt.decode(access_token, settings.SECRET_KEY, algorithms=[settings.ALGORITHM])
        account: str | None = payload.get("sub")
        if account is None:
            raise UnauthorizedException(detail="Could not validate credentials")
    except jwt.PyJWTError as e:
        raise UnauthorizedException(detail="Could not validate credentials") from e

    return await service.get_user_by_account(account)


async def get_current_active_user(
    current_user: Annotated[User, Depends(get_current_user)],
) -> User:
    if current_user.is_disabled:
        raise UnauthorizedException(detail="User account is disabled")
    return current_user
