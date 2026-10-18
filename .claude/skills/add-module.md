---
name: add-module
description: Create a new Go backend module as an empty but working user-owned resource (table with only the standard columns, sqlc queries, service, httpapi handlers, OpenAPI paths, migration, integration tests)
user_invocable: true
---

# /add-module <name>

Create a new module under `backend/internal/modules/<name>/` that already works end to end: a table with only the
standard columns (`id`, `owner_id`, `created_at`, `updated_at`), create, list, get and delete operations for the
signed-in user's rows, a generated migration, integration tests, and regenerated frontend types. The user then adds
domain columns. When the fields are already known, use `/add-crud <entity> --fields "..."` instead: it generates
the same module with the fields and PATCH included.

## Instructions

Read `.claude/skills/add-crud.md` and follow it from start to finish with an empty field list. Its rules, names
(`<pkg>`, `<table>`, `<path>`, `<Entity>`, `<Entities>`), file layout, generate and verify steps all apply. With
no fields, these parts change:

- **Arguments**: `<name>` is the module name; derive the placeholders from it as add-crud step 1 does (for
  `widgets`: `<pkg>` `widgets`, `<table>` `widgets`, `<path>` `widgets`, `<Entity>` `Widget`).
- **`schema.sql`**: the table has `id`, `owner_id`, `created_at`, `updated_at` and the
  `<table>_owner_id_id_idx` index; nothing else.
- **`queries/<table>.sql`**: `Create<Entity>` is
  `INSERT INTO <table> (owner_id) VALUES (@owner_id) RETURNING id, owner_id, created_at, updated_at;`. With a single
  parameter sqlc generates `Create<Entity>(ctx, ownerID uuid.UUID)` instead of a params struct. `Get<Entity>`,
  `List<Entities>` and `Delete<Entity>` are as in add-crud. There is no `Update<Entity>`: with no fields there is
  nothing to patch, and an UPDATE that only sets `updated_at` would be dead code. The file's header comment says
  that deletes (not updates) report the rows they touched.
- **`<pkg>.go`**: no `Patch` type and no `Update` method; `Create(ctx)` takes no field parameters.
- **Contract**: `/<path>` has `post` and `get`, `/<path>/{id}` has `get` and `delete`, with the same responses as
  the items operations of the same method. There is no `patch` and no `<Entity>Patch` schema.
  `Create<Entity>Request` is `type: object` with `additionalProperties: false` and no properties, so clients send
  `{}` and later optional fields can be added without a breaking change. `<Entity>` lists `id`, `owner_id`,
  `created_at`, `updated_at` as required.
- **Migration**: `make -C backend gen sqlc migration name=add_<table>`.
- **Handlers**: no `Update<Entity>`. `Create<Entity>` calls `s.<pkg>.Create(ctx)` and names its request
  parameter `_`, since it reads nothing from it (oapi-codegen generates `map[string]interface{}` for the empty
  object). `to<Entity>` sets only `Id`, `OwnerId`,
  `CreatedAt`, `UpdatedAt`. Wire `Deps`, `apiServer`, `cmd/app/main.go` and `newAuthAPI` exactly as add-crud step
  6 says.
- **Tests** (`backend/internal/httpapi/<pkg>_test.go`, following `items_test.go`): create returns 201 with
  `Location` and the caller as `owner_id`, and 401 without a session; an unknown property (`{"name":"x"}`) is 422
  `additional_property`; list returns only the caller's rows newest first and pages with `?limit=2` over 3 rows;
  get returns 200, 404 for an unknown id and 404 for another user's row; delete returns 204 with an empty body, then
  404, and another user's delete is 404 and leaves the row. Skip the add-crud cases about fields and PATCH.

Finish with add-crud steps 8 and 9 (`pnpm -C frontend gen:api`, `make -C backend fmt`, `make -C backend check`,
`pnpm -C frontend lint`, `pnpm -C frontend test`) and report each result.

## Tell the user what comes next

- Add columns in `schema.sql` using add-crud's field type mapping (OpenAPI schema, CHECK, Go types), extend the
  queries, service, contract, handlers and tests to match, and add the PATCH operation following add-crud, then
  generate a new migration with `make -C backend migration name=<x>`.
- Until this migration is merged into `main`, it may be regenerated (delete the file, restore `atlas.sum`, run the
  target again). After that it is immutable, and a new `NOT NULL` column on a table with rows needs a constant default
  (a volatile one such as `uuidv7()` needs an expand step); squawk flags the unsafe forms.
