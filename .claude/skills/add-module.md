---
name: add-module
description: Create a new Go backend module as an empty but working user-owned resource (proto service with only the standard fields, table, sqlc queries, service, Connect handler, migration, integration tests)
user_invocable: true
---

# /add-module <name>

Create a new module under `backend/internal/modules/<name>/` that already works end to end: a proto service
`<Entity>AppService` in `api/app/<name>/v1/<name>.proto` with create, list, get and delete for the signed-in user's
rows, a table with only the standard columns (`id`, `owner_id`, `created_at`, `updated_at`), a Connect handler, a
generated migration, integration tests, and the regenerated Go and TypeScript code. The user then adds domain
fields. When the fields are already known, use `/add-crud <entity> --fields "..."` instead: it generates the same
module with the fields and Update included.

## Instructions

Read `.claude/skills/add-crud.md` and follow it from start to finish with an empty field list. Its rules, names
(`<pkg>`, `<table>`, `<Entity>`, `<Entities>`, `<entity_snake>`), file layout, generate and verify steps all apply.
With no fields, these parts change:

- **Arguments**: `<name>` is the module name; derive the placeholders from it as add-crud step 1 does (for
  `widgets`: `<pkg>` `widgets`, `<table>` `widgets`, `<Entity>` `Widget`, `<entity_snake>` `widget`).
- **`schema.sql`**: the table has `id`, `owner_id`, `created_at`, `updated_at` and the
  `<table>_owner_id_id_idx` index; nothing else.
- **`queries/<table>.sql`**: `Create<Entity>` is
  `INSERT INTO <table> (owner_id) VALUES (@owner_id) RETURNING id, owner_id, created_at, updated_at;`. With a single
  parameter sqlc generates `Create<Entity>(ctx, ownerID uuid.UUID)` instead of a params struct. `Get<Entity>`,
  `List<Entities>` and `Delete<Entity>` are as in add-crud. There is no `Update<Entity>`: with no fields there is
  nothing to change, and an UPDATE that only sets `updated_at` would be dead code. The file's header comment says
  that deletes (not updates) report the rows they touched.
- **`<pkg>.go`**: no limits, no `Patch` type and no `Update` method; `Create(ctx)` takes no field parameters.
- **Contract**: `<Entity>` has `id`, `owner_id`, `create_time`, `update_time`. `<Entity>AppService` has
  `Create<Entity>`, `Get<Entity>`, `List<Entities>` and `Delete<Entity>`, with the same requests and responses as
  add-crud step 4; there is no `Update<Entity>`. `Create<Entity>Request` keeps `<Entity> <entity_snake> = 1;` even
  though the server reads nothing from it yet, so fields added later need no new request field.
- **Generate**: `make -C backend gen sqlc`, then `make generate-migration MSG=add_<table>` from the repository root.
- **Handler**: no `<entity>Writable` and no `Update<Entity>`. `Create<Entity>` calls `h.<pkg>.Create(ctx)` and names
  its request parameter `_`. `to<Entity>` sets only `Id`, `OwnerId`, `CreateTime`, `UpdateTime`. Wire `Deps`,
  `connectServices`, `cmd/app/main.go` and `newAuthAPI` exactly as add-crud step 6 says.
- **Tests** (`backend/internal/httpapi/<pkg>_test.go`, following `items_test.go`): create returns the resource with
  the caller as `owner_id` and Get returns the same one; every method without a session is `unauthenticated` before
  decoding; list returns only the caller's rows newest first and pages with `page_size: 2` over 3 rows; get of an
  unknown id and of another user's row is `not_found`; delete succeeds, then get and delete are `not_found`, and
  another user's delete is `not_found` and leaves the row. Skip the add-crud cases about fields and Update. Update
  the backend test count in `scripts/quality-locks.json`.

Finish with add-crud steps 8 and 9 (`pnpm -C frontend gen:api`, `make -C backend fmt`, `make -C backend check`,
`pnpm -C frontend gen:api:check`, `pnpm -C frontend lint`, `pnpm -C frontend test`) and report each result.

## Tell the user what comes next

- Add fields with add-crud's field type mapping (proto field, column and CHECK, Go types, handler conversion): the
  column in `schema.sql`, the field in the proto, the limits in the service with `apperr.Invalid`, the queries,
  handler and tests to match, and the `Update<Entity>` method with `update_mask` following add-crud; then
  `make -C backend gen sqlc` and a new migration with `make generate-migration MSG=<name>`.
- Until this module is merged into `main`, the proto and the migration may change freely (for the migration: delete
  the file, restore `atlas.sum`, run the target again). After that the migration is immutable, the proto may only
  grow (`make -C backend api-breaking`), and a new `NOT NULL` column on a table with rows needs a constant default
  (a volatile one such as `uuidv7()` needs an expand step); squawk flags the unsafe forms.
