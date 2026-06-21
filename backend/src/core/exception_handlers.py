from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from src.core.exceptions import AppException
from src.core.logging import logger


def register_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(AppException)
    async def app_exception_handler(request: Request, exc: AppException) -> JSONResponse:
        logger.warning("app_exception", detail=exc.detail, status_code=exc.status_code)
        return JSONResponse(
            status_code=exc.status_code,
            content={"detail": exc.detail},
        )

    @app.exception_handler(RequestValidationError)
    async def validation_exception_handler(
        request: Request, exc: RequestValidationError
    ) -> JSONResponse:
        field_errors = {}
        for err in exc.errors():
            loc_path = err["loc"]
            # Strip input location (like 'body', 'query') if it has sub-fields
            if len(loc_path) > 1:
                field_name = ".".join(str(part) for part in loc_path[1:])
            elif len(loc_path) == 1:
                field_name = str(loc_path[0])
            else:
                field_name = "global"

            if field_name not in field_errors:
                field_errors[field_name] = []
            field_errors[field_name].append(err["msg"])

        return JSONResponse(
            status_code=422,
            content={
                "detail": "Validation failed",
                "errors": field_errors,
            },
        )

    @app.exception_handler(Exception)
    async def unhandled_exception_handler(request: Request, exc: Exception) -> JSONResponse:
        logger.exception("unhandled_exception", exc_info=exc)
        return JSONResponse(
            status_code=500,
            content={"detail": "Internal server error"},
        )
