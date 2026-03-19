from datetime import UTC, datetime, timedelta

from src.auth.models import RefreshToken, User
from src.core.logging import logger
from src.auth.repository import RefreshTokenRepository, UserRepository
from src.auth.schemas import UserCreateInput
from src.core.config import settings
from src.core.exceptions import (
    AlreadyExistsException,
    NotFoundException,
    UnauthorizedException,
)
from src.core.security import (
    create_access_token,
    create_refresh_token,
    get_password_hash,
    verify_password,
)


class AuthService:
    def __init__(self, repo: UserRepository, refresh_repo: RefreshTokenRepository):
        self.repo = repo
        self.refresh_repo = refresh_repo

    async def register(self, data: UserCreateInput) -> dict:
        existing = await self.repo.get_by_account(data.account)
        if existing:
            raise AlreadyExistsException(detail="User already exists")

        user = User(
            account=data.account,
            password=get_password_hash(data.password),
        )
        await self.repo.create(user)
        logger.info("user_registered", account=data.account)

        return await self._create_token_pair(user)

    # Dummy hash used to prevent timing-based account enumeration
    _DUMMY_HASH = get_password_hash("dummy-password-for-timing")

    async def login(self, account: str, password: str) -> dict:
        user = await self.repo.get_by_account(account)

        # Always run verify_password to prevent timing-based account enumeration
        stored_hash = user.password if user else self._DUMMY_HASH
        password_valid = verify_password(password, stored_hash)

        if not user or not password_valid or user.is_disabled:
            logger.warning("login_failed", account=account)
            raise UnauthorizedException(detail="Invalid credentials")

        logger.info("login_success", account=account)
        return await self._create_token_pair(user)

    async def refresh(self, raw_refresh_token: str) -> dict:
        token = await self.refresh_repo.get_by_token(raw_refresh_token)
        if not token:
            raise UnauthorizedException(detail="Invalid refresh token")

        if token.expires_at < datetime.now(UTC):
            raise UnauthorizedException(detail="Refresh token expired")

        # Revoke old token and issue new pair
        await self.refresh_repo.revoke(token)
        user = await self.repo.get_by_id(token.user_id)
        if not user or user.is_disabled:
            raise UnauthorizedException(detail="Invalid credentials")

        logger.info("token_refreshed", user_id=user.id)
        return await self._create_token_pair(user)

    async def logout(self, raw_refresh_token: str) -> None:
        token = await self.refresh_repo.get_by_token(raw_refresh_token)
        if token:
            await self.refresh_repo.revoke(token)

    async def get_user_by_account(self, account: str) -> User:
        user = await self.repo.get_by_account(account)
        if not user:
            raise NotFoundException(detail="User not found")
        return user

    async def _create_token_pair(self, user: User) -> dict:
        access_token = create_access_token(
            data={"sub": user.account},
            expires_delta=timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES),
        )

        raw_token, token_hash = create_refresh_token()
        refresh_token = RefreshToken(
            token_hash=token_hash,
            user_id=user.id,
            expires_at=datetime.now(UTC) + timedelta(days=settings.REFRESH_TOKEN_EXPIRE_DAYS),
        )
        await self.refresh_repo.create(refresh_token)

        return {
            "access_token": access_token,
            "refresh_token": raw_token,
            "token_type": "bearer",
        }
