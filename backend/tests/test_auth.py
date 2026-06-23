import uuid

import jwt
import pytest
from httpx import AsyncClient

from src.core.config import settings


@pytest.mark.asyncio
async def test_register_sets_cookies(client: AsyncClient):
    account = f"newuser_{uuid.uuid4().hex[:8]}"
    resp = await client.post(
        "/auth/register",
        json={"account": account, "password": "ValidPass1"},
    )
    assert resp.status_code == 200
    assert resp.json()["data"]["account"] == account
    # tokens are delivered as httpOnly cookies, not in the response body
    assert resp.cookies.get("access_token")
    assert resp.cookies.get("refresh_token")
    assert "access_token" not in resp.text


@pytest.mark.asyncio
async def test_register_duplicate(client: AsyncClient):
    account = f"dupuser_{uuid.uuid4().hex[:8]}"
    data = {"account": account, "password": "ValidPass1"}
    assert (await client.post("/auth/register", json=data)).status_code == 200
    assert (await client.post("/auth/register", json=data)).status_code == 409


@pytest.mark.asyncio
async def test_register_weak_password(client: AsyncClient):
    resp = await client.post(
        "/auth/register",
        json={"account": "weakpw", "password": "short"},
    )
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_register_no_uppercase(client: AsyncClient):
    resp = await client.post(
        "/auth/register",
        json={"account": "noupperuser", "password": "alllower1"},
    )
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_login_success(client: AsyncClient):
    account = f"loginuser_{uuid.uuid4().hex[:8]}"
    await client.post("/auth/register", json={"account": account, "password": "ValidPass1"})
    resp = await client.post(
        "/auth/login",
        data={"username": account, "password": "ValidPass1"},
    )
    assert resp.status_code == 200
    assert resp.cookies.get("access_token")
    assert resp.cookies.get("refresh_token")


@pytest.mark.asyncio
async def test_login_wrong_password(client: AsyncClient):
    account = f"wrongpw_{uuid.uuid4().hex[:8]}"
    await client.post("/auth/register", json={"account": account, "password": "ValidPass1"})
    resp = await client.post(
        "/auth/login",
        data={"username": account, "password": "WrongPass1"},
    )
    assert resp.status_code == 401
    assert resp.json()["detail"] == "Invalid credentials"


@pytest.mark.asyncio
async def test_login_nonexistent_user(client: AsyncClient):
    resp = await client.post(
        "/auth/login",
        data={"username": "noone", "password": "Whatever1"},
    )
    assert resp.status_code == 401
    assert resp.json()["detail"] == "Invalid credentials"


@pytest.mark.asyncio
async def test_refresh_rotates_and_revokes_old_token(client: AsyncClient):
    account = f"refresh_{uuid.uuid4().hex[:8]}"
    reg = await client.post("/auth/register", json={"account": account, "password": "ValidPass1"})
    old_refresh = reg.cookies.get("refresh_token")
    client.cookies.clear()  # control exactly which refresh token is sent
    old_cookie = {"Cookie": f"refresh_token={old_refresh}"}

    r1 = await client.post("/auth/refresh", headers=old_cookie)
    assert r1.status_code == 200
    assert r1.cookies.get("access_token")
    client.cookies.clear()

    # Old refresh token was revoked on rotation -> replay is rejected
    r2 = await client.post("/auth/refresh", headers=old_cookie)
    assert r2.status_code == 401


@pytest.mark.asyncio
async def test_refresh_without_cookie(client: AsyncClient):
    resp = await client.post("/auth/refresh")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_invalid_refresh_token(client: AsyncClient):
    resp = await client.post(
        "/auth/refresh", headers={"Cookie": "refresh_token=totally-invalid-token"}
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_logout_clears_session(auth_client: AsyncClient):
    assert (await auth_client.get("/auth/me")).status_code == 200
    resp = await auth_client.post("/auth/logout")
    assert resp.status_code == 200
    # cookies cleared -> protected route no longer accessible
    assert (await auth_client.get("/auth/me")).status_code == 401


@pytest.mark.asyncio
async def test_me_authenticated(auth_client: AsyncClient):
    resp = await auth_client.get("/auth/me")
    assert resp.status_code == 200
    data = resp.json()["data"]
    assert "account" in data
    assert "id" in data


@pytest.mark.asyncio
async def test_me_unauthenticated(client: AsyncClient):
    resp = await client.get("/auth/me")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_me_via_bearer_header(client: AsyncClient):
    """Cookie is primary, but the Authorization header still works (Swagger/clients)."""
    account = f"bearer_{uuid.uuid4().hex[:8]}"
    reg = await client.post("/auth/register", json={"account": account, "password": "ValidPass1"})
    token = reg.cookies.get("access_token")
    client.cookies.clear()  # drop the cookie so only the header can authenticate
    resp = await client.get("/auth/me", headers={"Authorization": f"Bearer {token}"})
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_expired_access_token(client: AsyncClient):
    expired_token = jwt.encode(
        {"sub": "testuser", "exp": 0},
        settings.SECRET_KEY,
        algorithm=settings.ALGORITHM,
    )
    resp = await client.get("/auth/me", headers={"Cookie": f"access_token={expired_token}"})
    assert resp.status_code == 401
