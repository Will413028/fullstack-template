import pytest
from httpx import AsyncClient


@pytest.mark.asyncio
async def test_create_item(auth_client: AsyncClient):
    resp = await auth_client.post(
        "/items",
        json={"title": "Test Item", "description": "A test item"},
    )
    assert resp.status_code == 200
    data = resp.json()["data"]
    assert data["title"] == "Test Item"
    assert data["description"] == "A test item"
    assert "id" in data
    assert "owner_id" in data


@pytest.mark.asyncio
async def test_create_item_unauthenticated(client: AsyncClient):
    resp = await client.post("/items", json={"title": "Test Item"})
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_list_my_items(auth_client: AsyncClient):
    await auth_client.post("/items", json={"title": "List Test"})
    resp = await auth_client.get("/items")
    assert resp.status_code == 200
    data = resp.json()
    assert "data" in data
    assert "total" in data
    assert "page" in data
    assert "size" in data
    assert "pages" in data
    assert len(data["data"]) >= 1


@pytest.mark.asyncio
async def test_get_item(auth_client: AsyncClient):
    create_resp = await auth_client.post("/items", json={"title": "Get Test"})
    item_id = create_resp.json()["data"]["id"]

    resp = await auth_client.get(f"/items/{item_id}")
    assert resp.status_code == 200
    assert resp.json()["data"]["title"] == "Get Test"


@pytest.mark.asyncio
async def test_update_item(auth_client: AsyncClient):
    create_resp = await auth_client.post("/items", json={"title": "Update Test"})
    item_id = create_resp.json()["data"]["id"]

    resp = await auth_client.patch(f"/items/{item_id}", json={"title": "Updated Title"})
    assert resp.status_code == 200
    assert resp.json()["data"]["title"] == "Updated Title"


@pytest.mark.asyncio
async def test_delete_item(auth_client: AsyncClient):
    create_resp = await auth_client.post("/items", json={"title": "Delete Test"})
    item_id = create_resp.json()["data"]["id"]

    resp = await auth_client.delete(f"/items/{item_id}")
    assert resp.status_code == 200

    resp = await auth_client.get(f"/items/{item_id}")
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_other_users_item(auth_client: AsyncClient, auth_client_2: AsyncClient):
    # User 1 creates an item
    create_resp = await auth_client.post("/items", json={"title": "Ownership Test"})
    item_id = create_resp.json()["data"]["id"]

    # User 2 (separate session) tries to update it
    resp = await auth_client_2.patch(f"/items/{item_id}", json={"title": "Hacked"})
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_pagination(auth_client: AsyncClient):
    for i in range(3):
        await auth_client.post("/items", json={"title": f"Page Test {i}"})

    resp = await auth_client.get("/items?page=1&size=2")
    assert resp.status_code == 200
    data = resp.json()
    assert data["size"] == 2
    assert data["page"] == 1
    assert len(data["data"]) <= 2
