import uuid

import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

# Import all models so Base.metadata knows about them for create_all
import src.auth.models  # noqa: F401
import src.items.models  # noqa: F401
from src.core.config import settings
from src.core.models.base import Base

_RUN_ID = uuid.uuid4().hex[:6]
TEST_USER = {"account": f"testuser_{_RUN_ID}", "password": "TestPass1"}
TEST_USER_2 = {"account": f"testuser2_{_RUN_ID}", "password": "TestPass2"}


@pytest_asyncio.fixture(scope="session")
async def _setup_db():
    """Create tables and reconfigure the app's engine for the test event loop."""
    from urllib.parse import urlparse, urlunparse

    from sqlalchemy import text

    import src.core.database as db_module

    # Dispose the engine created at import time (wrong event loop)
    await db_module.engine.dispose()

    base_url = settings.async_database_url
    parsed = urlparse(base_url)
    db_name = parsed.path.lstrip("/")

    # If the database name already ends with _test or is testdb, use it directly.
    # Otherwise, derive the test database URL and dynamically create the database.
    if db_name.endswith("_test") or db_name == "testdb" or not db_name:
        test_db_url = base_url
    else:
        test_db_name = f"{db_name}_test"
        test_db_url = urlunparse(parsed._replace(path=f"/{test_db_name}"))

        # Connect to 'postgres' database to check and create the test database
        postgres_url = urlunparse(parsed._replace(path="/postgres"))
        temp_engine = create_async_engine(postgres_url, isolation_level="AUTOCOMMIT")
        try:
            async with temp_engine.connect() as conn:
                result = await conn.execute(
                    text(f"SELECT 1 FROM pg_database WHERE datname='{test_db_name}'")
                )
                if not result.scalar():
                    await conn.execute(text(f"CREATE DATABASE {test_db_name}"))
        except Exception:
            # Fallback in case of lack of permissions to connect to default DB or create DB.
            # We let the subsequent connection to test_db_url fail naturally if the DB is missing.
            pass
        finally:
            await temp_engine.dispose()

    # Create a new engine in the test event loop
    engine = create_async_engine(
        test_db_url,
        echo=False,
        pool_size=5,
        max_overflow=0,
    )
    session_factory = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)

    # Patch the app's database module
    db_module.engine = engine
    db_module.async_session_factory = session_factory

    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    yield engine

    # Drop all tables to clean up after test run
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

    await engine.dispose()


@pytest_asyncio.fixture
async def client(_setup_db):
    from src.main import app

    # Disable rate limiting in tests
    if hasattr(app.state, "limiter") and app.state.limiter:
        app.state.limiter.enabled = False

    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac


@pytest_asyncio.fixture
async def auth_tokens(client: AsyncClient) -> dict:
    resp = await client.post("/auth/register", json=TEST_USER)
    if resp.status_code == 409:
        resp = await client.post(
            "/auth/login",
            data={"username": TEST_USER["account"], "password": TEST_USER["password"]},
        )
    assert resp.status_code == 200, f"Auth setup failed: {resp.status_code} {resp.text}"
    return resp.json()


@pytest_asyncio.fixture
async def auth_header(auth_tokens: dict) -> dict:
    return {"Authorization": f"Bearer {auth_tokens['access_token']}"}


@pytest_asyncio.fixture
async def auth_tokens_2(client: AsyncClient) -> dict:
    resp = await client.post("/auth/register", json=TEST_USER_2)
    if resp.status_code == 409:
        resp = await client.post(
            "/auth/login",
            data={"username": TEST_USER_2["account"], "password": TEST_USER_2["password"]},
        )
    assert resp.status_code == 200, f"Auth setup failed: {resp.status_code} {resp.text}"
    return resp.json()


@pytest_asyncio.fixture
async def auth_header_2(auth_tokens_2: dict) -> dict:
    return {"Authorization": f"Bearer {auth_tokens_2['access_token']}"}
