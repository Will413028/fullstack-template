from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from src.auth.router import router as auth_router
from src.core.config import settings
from src.core.exception_handlers import register_exception_handlers
from src.core.logging import logger
from src.core.middleware import ProcessTimeMiddleware, RequestIdMiddleware
from src.health.router import router as health_router
from src.items.router import router as items_router


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info("app_startup", app_name=settings.APP_NAME)
    yield
    logger.info("app_shutdown")


def create_app() -> FastAPI:
    app = FastAPI(
        title=settings.APP_NAME,
        version=settings.APP_VERSION,
        docs_url="/docs" if settings.MODE == "dev" else None,
        redoc_url="/redoc" if settings.MODE == "dev" else None,
        openapi_url="/openapi.json" if settings.MODE == "dev" else None,
        lifespan=lifespan,
        openapi_tags=[
            {"name": "health", "description": "Health check endpoints"},
            {"name": "auth", "description": "Authentication & user management"},
            {"name": "items", "description": "Item CRUD operations"},
        ],
    )

    # Middleware (order matters: first added = outermost)
    app.add_middleware(RequestIdMiddleware)
    app.add_middleware(ProcessTimeMiddleware)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.CORS_ORIGINS,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # Exception handlers
    register_exception_handlers(app)

    # Routers
    app.include_router(health_router)
    app.include_router(auth_router)
    app.include_router(items_router)

    return app


app = create_app()
