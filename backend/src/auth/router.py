from typing import Annotated

from fastapi import APIRouter, Depends, Request, Response
from fastapi.security import OAuth2PasswordRequestForm
from slowapi import Limiter
from slowapi.util import get_remote_address

from src.auth.cookies import REFRESH_COOKIE, clear_auth_cookies, set_auth_cookies
from src.auth.dependencies import get_auth_service, get_current_active_user
from src.auth.models import User
from src.auth.schemas import UserCreateInput, UserResponse
from src.auth.service import AuthService
from src.core.exceptions import UnauthorizedException
from src.core.schemas.base import DataResponse, DetailResponse

limiter = Limiter(key_func=get_remote_address)
router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/register")
@limiter.limit("5/minute")
async def register(
    request: Request,
    response: Response,
    data: UserCreateInput,
    service: AuthService = Depends(get_auth_service),
) -> DataResponse[UserResponse]:
    user, tokens = await service.register(data)
    set_auth_cookies(response, tokens["access_token"], tokens["refresh_token"])
    return DataResponse(data=UserResponse.model_validate(user))


@router.post("/login")
@limiter.limit("5/minute")
async def login(
    request: Request,
    response: Response,
    form_data: Annotated[OAuth2PasswordRequestForm, Depends()],
    service: AuthService = Depends(get_auth_service),
) -> DataResponse[UserResponse]:
    user, tokens = await service.login(form_data.username, form_data.password)
    set_auth_cookies(response, tokens["access_token"], tokens["refresh_token"])
    return DataResponse(data=UserResponse.model_validate(user))


@router.post("/refresh")
@limiter.limit("5/minute")
async def refresh(
    request: Request,
    response: Response,
    service: AuthService = Depends(get_auth_service),
) -> DataResponse[UserResponse]:
    raw_refresh_token = request.cookies.get(REFRESH_COOKIE)
    if not raw_refresh_token:
        raise UnauthorizedException(detail="Missing refresh token")
    user, tokens = await service.refresh(raw_refresh_token)
    set_auth_cookies(response, tokens["access_token"], tokens["refresh_token"])
    return DataResponse(data=UserResponse.model_validate(user))


@router.post("/logout")
@limiter.limit("5/minute")
async def logout(
    request: Request,
    response: Response,
    service: AuthService = Depends(get_auth_service),
) -> DetailResponse:
    raw_refresh_token = request.cookies.get(REFRESH_COOKIE)
    if raw_refresh_token:
        await service.logout(raw_refresh_token)
    clear_auth_cookies(response)
    return DetailResponse(detail="Logged out")


@router.get("/me")
async def get_me(
    current_user: User = Depends(get_current_active_user),
) -> DataResponse[UserResponse]:
    return DataResponse(data=UserResponse.model_validate(current_user))
