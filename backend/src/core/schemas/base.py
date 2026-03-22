from typing import Generic, TypeVar

from pydantic import BaseModel, Field

T = TypeVar("T")


class DataResponse(BaseModel, Generic[T]):
    data: T


class PaginatedResponse(BaseModel, Generic[T]):
    data: list[T]
    total: int = Field(..., examples=[100])
    page: int = Field(..., examples=[1])
    size: int = Field(..., examples=[20])
    pages: int = Field(..., examples=[5])


class DetailResponse(BaseModel):
    detail: str = Field(..., examples=["successful"])
