import pytest
from httpx import AsyncClient


@pytest.mark.asyncio
async def test_create_item(client: AsyncClient, auth_header):
    resp = await client.post(
        "/items",
        json={"title": "Test Item", "description": "A test item"},
        headers=auth_header,
    )
    assert resp.status_code == 200
    data = resp.json()["data"]
    assert data["title"] == "Test Item"
    assert data["description"] == "A test item"
    assert "id" in data
    assert "owner_id" in data


@pytest.mark.asyncio
async def test_create_item_unauthenticated(client: AsyncClient):
    resp = await client.post(
        "/items",
        json={"title": "Test Item"},
    )
    assert resp.status_code == 401


@pytest.mark.asyncio
async def test_list_my_items(client: AsyncClient, auth_header):
    # Create an item first
    await client.post(
        "/items",
        json={"title": "List Test"},
        headers=auth_header,
    )
    resp = await client.get("/items", headers=auth_header)
    assert resp.status_code == 200
    data = resp.json()
    assert "data" in data
    assert "total" in data
    assert "page" in data
    assert "size" in data
    assert "pages" in data
    assert len(data["data"]) >= 1


@pytest.mark.asyncio
async def test_get_item(client: AsyncClient, auth_header):
    # Create
    create_resp = await client.post(
        "/items",
        json={"title": "Get Test"},
        headers=auth_header,
    )
    item_id = create_resp.json()["data"]["id"]

    # Get (requires auth)
    resp = await client.get(f"/items/{item_id}", headers=auth_header)
    assert resp.status_code == 200
    assert resp.json()["data"]["title"] == "Get Test"


@pytest.mark.asyncio
async def test_update_item(client: AsyncClient, auth_header):
    # Create
    create_resp = await client.post(
        "/items",
        json={"title": "Update Test"},
        headers=auth_header,
    )
    item_id = create_resp.json()["data"]["id"]

    # Update
    resp = await client.patch(
        f"/items/{item_id}",
        json={"title": "Updated Title"},
        headers=auth_header,
    )
    assert resp.status_code == 200
    assert resp.json()["data"]["title"] == "Updated Title"


@pytest.mark.asyncio
async def test_delete_item(client: AsyncClient, auth_header):
    # Create
    create_resp = await client.post(
        "/items",
        json={"title": "Delete Test"},
        headers=auth_header,
    )
    item_id = create_resp.json()["data"]["id"]

    # Delete
    resp = await client.delete(f"/items/{item_id}", headers=auth_header)
    assert resp.status_code == 200

    # Verify deleted
    resp = await client.get(f"/items/{item_id}", headers=auth_header)
    assert resp.status_code == 404


@pytest.mark.asyncio
async def test_update_other_users_item(client: AsyncClient, auth_header, auth_header_2):
    # User 1 creates item
    create_resp = await client.post(
        "/items",
        json={"title": "Ownership Test"},
        headers=auth_header,
    )
    item_id = create_resp.json()["data"]["id"]

    # User 2 tries to update
    resp = await client.patch(
        f"/items/{item_id}",
        json={"title": "Hacked"},
        headers=auth_header_2,
    )
    assert resp.status_code == 403


@pytest.mark.asyncio
async def test_pagination(client: AsyncClient, auth_header):
    # Create multiple items
    for i in range(3):
        await client.post(
            "/items",
            json={"title": f"Page Test {i}"},
            headers=auth_header,
        )

    resp = await client.get("/items?page=1&size=2", headers=auth_header)
    assert resp.status_code == 200
    data = resp.json()
    assert data["size"] == 2
    assert data["page"] == 1
    assert len(data["data"]) <= 2
