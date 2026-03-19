---
name: add-crud
description: Generate a complete CRUD module with specified fields
user_invocable: true
---

# /add-crud <entity> --fields "<field_definitions>"

Generate a complete CRUD module with all fields filled in, ready to use.

## Instructions

Parse the user's arguments:
- `<entity>` — the entity name (e.g., `product`, `order`)
- `--fields` — field definitions in format: `"name:type, description:text?, price:float, category_id:int"`

### Field Type Mapping

| User Type | Python Type | SQLAlchemy Column |
|-----------|-------------|-------------------|
| `str` or `string` | `str` | `String(255)` |
| `text` | `str` | `Text` |
| `int` or `integer` | `int` | `Integer` |
| `float` or `decimal` | `float` | `Float` |
| `bool` or `boolean` | `bool` | `Boolean` |
| `date` | `datetime.date` | `Date` |
| `datetime` | `datetime.datetime` | `DateTime(timezone=True)` |

A trailing `?` means the field is optional/nullable (e.g., `description:text?`).

### Steps

1. Use the same file structure as `/add-module`, but with all fields populated based on the `--fields` argument.

2. For the **model**, map each field to the correct SQLAlchemy type. Optional fields get `nullable=True` and `Mapped[type | None]`.

3. For **schemas**:
   - `Create` schema: includes all non-optional fields as required, optional fields as `Optional`
   - `Update` schema: ALL fields are `Optional` (partial update)
   - `Response` schema: includes all fields + `id`

4. If `--owner` flag is provided, add `owner_id: Mapped[int]` ForeignKey to the model, and add ownership checking in the service (same pattern as `backend/src/items/` module).

**Note:** All backend files live under `backend/`. Create files at `backend/src/<name>/`, register in `backend/src/main.py`, and register models in `backend/alembic/env.py`.

5. Register everything (router in `backend/src/main.py`, model in `backend/alembic/env.py`).

6. Remind the user to run `cd backend && make generate_migration && make migration`.
