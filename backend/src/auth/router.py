from typing import Annotated

from fastapi import APIRouter, Depends
from fastapi.security import OAuth2PasswordRequestForm

from src.auth.dependencies import get_auth_service, get_current_active_user
from src.auth.models import User
from src.auth.schemas import Token, UserCreateInput, UserInfo, UserResponse
from src.auth.service import AuthService
from src.core.schemas.base import DataResponse

router = APIRouter(prefix="/auth", tags=["auth"])


@router.post("/register", response_model=Token)
async def register(
    data: UserCreateInput,
    service: AuthService = Depends(get_auth_service),
):
    access_token = await service.register(data)
    return Token(access_token=access_token)


@router.post("/login", response_model=Token)
async def login(
    form_data: Annotated[OAuth2PasswordRequestForm, Depends()],
    service: AuthService = Depends(get_auth_service),
):
    access_token = await service.login(form_data.username, form_data.password)
    return Token(access_token=access_token)


@router.get("/me", response_model=DataResponse[UserResponse])
async def get_me(
    current_user: User = Depends(get_current_active_user),
):
    return DataResponse(data=UserResponse.model_validate(current_user))


@router.get("/me/info", response_model=UserInfo)
async def get_user_info(
    current_user: User = Depends(get_current_active_user),
):
    return UserInfo(name=current_user.account)
