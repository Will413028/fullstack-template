from typing import Annotated

from fastapi import APIRouter, Depends, Request
from fastapi.security import OAuth2PasswordRequestForm
from slowapi import Limiter
from slowapi.util import get_remote_address

from src.auth.dependencies import get_auth_service, get_current_active_user
from src.auth.models import User
from src.auth.schemas import RefreshTokenInput, TokenPair, UserCreateInput, UserResponse
from src.auth.service import AuthService
from src.core.schemas.base import DataResponse, DetailResponse

limiter = Limiter(key_func=get_remote_address)
router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/register")
@limiter.limit("5/minute")
async def register(
    request: Request,
    data: UserCreateInput,
    service: AuthService = Depends(get_auth_service),
) -> TokenPair:
    return await service.register(data)


@router.post("/login")
@limiter.limit("5/minute")
async def login(
    request: Request,
    form_data: Annotated[OAuth2PasswordRequestForm, Depends()],
    service: AuthService = Depends(get_auth_service),
) -> TokenPair:
    return await service.login(form_data.username, form_data.password)


@router.post("/refresh")
@limiter.limit("5/minute")
async def refresh(
    request: Request,
    body: RefreshTokenInput,
    service: AuthService = Depends(get_auth_service),
) -> TokenPair:
    return await service.refresh(body.refresh_token)


@router.post("/logout")
@limiter.limit("5/minute")
async def logout(
    request: Request,
    body: RefreshTokenInput,
    service: AuthService = Depends(get_auth_service),
) -> DetailResponse:
    await service.logout(body.refresh_token)
    return DetailResponse(detail="Logged out")


@router.get("/me")
async def get_me(
    current_user: User = Depends(get_current_active_user),
) -> DataResponse[UserResponse]:
    return DataResponse(data=UserResponse.model_validate(current_user))
