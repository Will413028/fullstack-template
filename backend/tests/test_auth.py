import uuid

import jwt
import pytest
from httpx import AsyncClient

from src.core.config import settings


@pytest.mark.asyncio
async def test_register_success(client: AsyncClient):
    unique_account = f"newuser_{uuid.uuid4().hex[:8]}"
    resp = await client.post(
        "/auth/register",
        json={"account": unique_account, "password": "ValidPass1"},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert "access_token" in data
    assert "refresh_token" in data
    assert data["token_type"] == "bearer"


@pytest.mark.asyncio
async def test_register_duplicate(client: AsyncClient):
    unique_account = f"dupuser_{uuid.uuid4().hex[:8]}"
    data = {"account": unique_account, "password": "ValidPass1"}
    resp1 = await client.post("/auth/register", json=data)
    assert resp1.status_code == 200
    resp2 = await client.post("/auth/register", json=data)
    assert resp2.status_code == 409


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
    data = resp.json()
    assert "access_token" in data
    assert "refresh_token" in data


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
async def test_refresh_token(client: AsyncClient, auth_tokens):
    resp = await client.post(
        "/auth/refresh",
        json={"refresh_token": auth_tokens["refresh_token"]},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert "access_token" in data
    assert "refresh_token" in data
    # Old refresh token should be revoked
    resp2 = await client.post(
        "/auth/refresh",
        json={"refresh_token": auth_tokens["refresh_token"]},
    )
    assert resp2.status_code == 401


@pytest.mark.asyncio
async def test_logout(client: AsyncClient, auth_tokens):
    resp = await client.post(
        "/auth/logout",
        json={"refresh_token": auth_tokens["refresh_token"]},
    )
    assert resp.status_code == 200


@pytest.mark.asyncio
async def test_me_authenticated(client: AsyncClient, auth_header):
    resp = await client.get("/auth/me", headers=auth_header)
    assert resp.status_code == 200
    data = resp.json()["data"]
    assert "account" in data
    assert "id" in data


@pytest.mark.asyncio
async def test_me_unauthenticated(client: AsyncClient):
    resp = await client.get("/auth/me")
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_expired_access_token(client: AsyncClient):
    expired_token = jwt.encode(
        {"sub": "testuser", "exp": 0},
        settings.SECRET_KEY,
        algorithm=settings.ALGORITHM,
    )
    resp = await client.get(
        "/auth/me",
        headers={"Authorization": f"Bearer {expired_token}"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_invalid_refresh_token(client: AsyncClient):
    resp = await client.post(
        "/auth/refresh",
        json={"refresh_token": "totally-invalid-token"},
    )
    assert resp.status_code == 401
