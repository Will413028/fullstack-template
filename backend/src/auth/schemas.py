from pydantic import BaseModel, ConfigDict


class UserCreateInput(BaseModel):
    account: str
    password: str


class Token(BaseModel):
    access_token: str
    token_type: str = "bearer"


class UserResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    account: str
    is_disabled: bool
    role: int


class UserInfo(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    name: str
