---
name: add-crud
description: Generate a complete user-owned CRUD resource (proto service in api/, schema.sql, sqlc queries, module service with the input limits, Connect handler, migration, integration tests) and regenerate the Go and TypeScript Connect code
user_invocable: true
---

# /add-crud <entity> --fields "<field_definitions>"

Generate a complete CRUD resource owned by the signed-in user: a proto service in `api/app/<pkg>/v1/<pkg>.proto`, a
new module under `backend/internal/modules/<pkg>/` (table, sqlc queries, service), a Connect handler in
`backend/internal/httpapi/`, a generated migration, integration tests through the generated Connect client, and the
regenerated frontend code. The reference implementation is the `items` module (`api/app/items/v1/items.proto`,
`backend/internal/modules/items/`, `backend/internal/httpapi/items.go`, `backend/internal/httpapi/items_test.go`);
when this file and `items` disagree, stop and tell the user instead of guessing. The rules are in
`docs/rules/api-design.md` and `docs/rules/database-design.md`.

## Rules the generated code must follow

- Modules never import `net/http`, `connectrpc.com/connect`, the generated code under `internal/gen`,
  `internal/httpapi`, `cmd/`, or another module (depguard in `backend/.golangci.yml` and `backend/internal/archtest`
  fail the build otherwise). Handlers live only in `backend/internal/httpapi`.
- SQL is written only in `queries/*.sql` and called only through the sqlc-generated `internal/store` package. Never
  edit generated files (`internal/modules/<pkg>/internal/store/*`, `backend/internal/gen/**`,
  `frontend/src/generated/**`).
- DDL is written only in the module's `schema.sql` (the desired state). Migrations are generated with
  `make generate-migration MSG=<name>` from the repository root (Atlas diff); never write or edit a migration by hand.
  `backend/atlas.hcl` globs `internal/modules/*/schema.sql` and loads auth first, so a new `schema.sql` is picked up
  without editing it.
- Primary key `id uuid PRIMARY KEY DEFAULT uuidv7()`; text columns are `text` with a CHECK that matches the service's
  limit; enums are `text` with `CHECK (col IN (...))`; times are `timestamptz`; every table has `created_at` and
  `updated_at timestamptz NOT NULL DEFAULT now()`, and every UPDATE query sets `updated_at = now()`.
- The resource is owned: `owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE`, deleted with the account
  (users is the only table another module may reference), and every query has `owner_id = @owner_id`. Another user's
  row is `not_found`, exactly like a missing one.
- Lengths, formats and enum values are checked once, in the module's service, with
  `apperr.Invalid(field, reason, detail)` (`docs/adr/2026-11-03-api-contract-protobuf-connect.md` D2).
  `reason` is the lowercase name of a value declared in `api/app/common/v1/errors.proto`'s `FieldViolationReason`
  (`required`, `too_short`, `too_long`, `pattern`, `out_of_range`, `invalid`); an undeclared reason reaches the client
  as `INVALID` and is logged. Do not write limits in the proto, the handler or frontend zod schemas.
- The service is `<Entity>AppService` (any signed-in user; the name gives the class, `rpc.ClassOf`). Only `Get*` and
  `List*` carry `option idempotency_level = NO_SIDE_EFFECTS;`. Create and Update return the resource and the package
  has its Get (`TestConnectServicesReturnTheirResources`). List pages with AIP-158 (`page_size`, `page_token`,
  `next_page_token`, newest first, no total); Update takes an AIP-134 `update_mask`.

If the user asks for something outside these rules (a resource not owned by a user, a public or admin service, a
reference to another module's table, a `decimal`/money or `date` field, a list sorted by something other than
creation time), stop and ask how they want it designed; this skill does not generate it.

## Step 1. Parse the arguments

- `<entity>`: the resource name, singular or plural, in snake_case (`note`, `notes`, `todo_list`). Derive the names
  below; if the plural is irregular or ambiguous, ask.

  | Placeholder | Rule | `notes` | `todo_list` |
  |---|---|---|---|
  | `<pkg>` | module directory, Go package and proto package segment: plural, lowercase, no separators | `notes` | `todolists` |
  | `<table>` | table name and the repeated field of the List response: plural snake_case | `notes` | `todo_lists` |
  | `<Entity>` / `<Entities>` | Go and proto names: PascalCase singular / plural | `Note` / `Notes` | `TodoList` / `TodoLists` |
  | `<entity>` / `<entities>` | camelCase singular / plural (test helpers, TS) | `note` / `notes` | `todoList` / `todoLists` |
  | `<entity_snake>` | proto field holding the resource: singular snake_case | `note` | `todo_list` |

  Stop if `backend/internal/modules/<pkg>` or `api/app/<pkg>` already exists.

- `--fields`: comma-separated `name:type`, each optionally followed by `required` or `optional` (a trailing `?` on
  the type also means optional). Fields are required unless marked optional. Names are snake_case; `id`, `owner_id`,
  `created_at`, `updated_at`, `create_time` and `update_time` are reserved (every resource gets them). Example:
  `"title:string(200) required, body:text optional, done:bool, priority:int?, status:enum(open|closed)"`.

## Step 2. Field type mapping

| User type | proto field | PostgreSQL column + CHECK | Go: sqlc store and service | Handler conversion | In items/auth |
|---|---|---|---|---|---|
| `string(N)` | `string` | `text` + `CHECK (char_length(f) BETWEEN 1 AND N)` | `string` | none | `Item.title` |
| `text` | `string` (no limit; the 1 MiB body limit caps it) | `text` | `string` | none | — |
| `int` | `int32` (JSON number) | `integer` | `int32` | none | `SignInLock.failures` (column `bigint`, converted) |
| `bigint` | `int64` (JSON string) | `bigint` | `int64` | none | — |
| `bool` | `optional bool` (see below) | `boolean NOT NULL` | `bool` | `proto.Bool(v)` out, `in.GetF()` in | `User.disabled` / `is_disabled` |
| `datetime` | `google.protobuf.Timestamp` named `<f>_time` (AIP-142) | `timestamptz` named `<f>_at` | `time.Time` | `timestamppb.New(t)` out, `ts.AsTime()` in | `create_time` / `created_at` |
| `enum(a\|b\|c)` | enum `<Entity><Field>` with `<ENTITY>_<FIELD>_UNSPECIFIED = 0`, `<ENTITY>_<FIELD>_A = 1`, ... | `text` + `CHECK (f IN ('a', 'b', 'c'))` | `string` constants in the module | a `map[string]<pkg>v1.<Entity><Field>` both ways | `Role` / `auth.RoleAdmin`, `roles` in `users.go` |

- Required fields are `NOT NULL`. Optional fields are nullable columns (no `NOT NULL`; a CHECK passes on NULL), `*T`
  in the store and service, and `optional T` in the proto, which protoc-gen-go generates as `*T` (`Item.Description`
  is `*string`). An optional `Timestamp` needs no `optional`: a nil message is unset.
- A `bool` is `optional bool` in the proto even when the column is `NOT NULL`: without a mask, `rpc.UpdatePaths`
  changes only the fields that have a value, and a plain `bool` set to `false` has none (the reason `User.disabled`
  is `optional bool`). The service treats an unset required bool on Create as `false`.
- A required `string(N)` is checked for `required` (empty) and `too_long`; a required enum reports `required` when
  it is `UNSPECIFIED` and `invalid` for a value outside the map.
- sqlc names follow Go initialisms (`ID`, `OwnerID`); protoc-gen-go does not (`Id`, `OwnerId`).

## Step 3. Module: `backend/internal/modules/<pkg>/`

### `schema.sql`

Columns in this order: `id`, the fields in the order given, `owner_id`, `created_at`, `updated_at`. Align and
comment like `items/schema.sql`.

```sql
-- <pkg> module: desired state of its table. Atlas generates migrations from this file (make generate-migration);
-- never edit applied migrations. Type rules follow docs/adr/2026-10-18-go-backend.md (UUIDv7 keys, text with CHECK,
-- timestamptz).

-- owner_id -> users.id is the only cross-module foreign key (backend/AGENTS.md); the <pkg> queries never read users.
-- <Entities> belong to their owner and are deleted with the account (docs/rules/database-design.md).
CREATE TABLE <table> (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  -- one line per field, from the mapping table, e.g.
  -- title    text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  -- body     text,
  owner_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Keyset pagination: WHERE owner_id = $1 AND id < $cursor ORDER BY id DESC.
CREATE INDEX <table>_owner_id_id_idx ON <table> (owner_id, id DESC);
```

### `sqlc.yaml`

Copy `backend/internal/modules/items/sqlc.yaml` and change the header comment's `items` to `<pkg>`. When a field is
an optional `datetime`, add the nullable override after the `timestamptz` one:

```yaml
          - db_type: timestamptz
            nullable: true
            go_type:
              type: time.Time
              pointer: true
```

### `queries/<table>.sql`

`<cols>` is the full column list in table order: `id, <fields...>, owner_id, created_at, updated_at`.

```sql
-- Every query is scoped to the owner (docs/adr/2026-10-18-api-and-auth-baseline.md D7): another user's row behaves
-- as if it did not exist. Updates and deletes report the rows they touched, so "not yours" is not_found rather than
-- a silent success.

-- name: Create<Entity> :one
INSERT INTO <table> (<fields...>, owner_id)
VALUES (@<field>..., @owner_id)
RETURNING <cols>;

-- name: Get<Entity> :one
SELECT <cols>
FROM <table>
WHERE id = @id AND owner_id = @owner_id;

-- List<Entities> returns the owner's rows newest first (UUIDv7 ids sort by creation time), after the cursor id when
-- one is given.
-- name: List<Entities> :many
SELECT <cols>
FROM <table>
WHERE owner_id = @owner_id AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT @row_limit;

-- Update<Entity> applies the fields update_mask selected: a null <required> leaves it unchanged, and
-- set_<optional> says whether <optional> was selected, so null can clear it.
-- name: Update<Entity> :one
UPDATE <table>
SET <required>   = coalesce(sqlc.narg('<required>'), <required>),
    <optional>   = CASE WHEN @set_<optional>::boolean THEN sqlc.narg('<optional>') ELSE <optional> END,
    updated_at   = now()
WHERE id = @id AND owner_id = @owner_id
RETURNING <cols>;

-- name: Delete<Entity> :execrows
DELETE FROM <table>
WHERE id = @id AND owner_id = @owner_id;
```

One `SET` line per field: the `coalesce` form for required fields, the `CASE` form for optional ones. Write the
comments with the resource's real names (as `items/queries/items.sql` does), never with placeholders left in.

### `<pkg>.go`

Copy the structure of `backend/internal/modules/items/items.go` and rename. The parts that depend on the fields:

- The package comment says what the resource is and that every query is scoped to the caller.
- `type <Entity> struct`: exactly the fields of the generated `store.<Entity>`, same names, types and order
  (`ID`, the fields, `OwnerID`, `CreatedAt`, `UpdatedAt`), so `<Entity>(row)` converts a row. Check against
  `internal/store/models.go` after `make -C backend sqlc`.
- `type Page struct { <Entities> []<Entity>; Next *uuid.UUID }`.
- `type Patch struct`: per required field `<Field> *T`; per optional field `Set<Field> bool` followed by
  `<Field> *T`. `empty()` returns true when every required pointer is nil and every `Set<Field>` is false. Document
  the meaning on the type as items does.
- The limits: one constant per limit (`max<Field>`, matching the schema's CHECK) and one `check<Field>` per checked
  field, returning `apperr.Invalid("<field>", "<reason>", "<developer text>")` with the module's own field name (the
  handler adds the request path). Lengths count code points (`utf8.RuneCountInString`). For an enum, declare the
  values as exported constants (`<Field><Value> = "<value>"`) and check membership. Several invalid fields are
  returned together with `errors.Join`. Create checks every field; Update checks the fields the patch sets.
- `Create(ctx, <field> T / *T ...)`: one parameter per field, in field order, passed to `store.Create<Entity>Params`
  together with `OwnerID`.
- `Get`, `List`, `Update`, `Delete`: as in items, with `errNotFound = apperr.New(apperr.NotFound, "", "<entity>
  not found")` and error prefixes `"<pkg>: create: %w"` and so on. `List(ctx, limit int, after *uuid.UUID)` passes
  `RowLimit: keyset.Rows(limit)` and cuts with `keyset.Trim(rows, limit, func(r store.<Entity>) uuid.UUID { return
  r.ID })` exactly as items does; `Update` returns `s.Get(ctx, id)` for an empty patch.
- `owner(ctx)` reads `authctx.From(ctx)` and returns an error (not an `apperr`) when no user is present: an
  AppService needs a session before the handler runs, so a missing user is a wiring bug.

## Step 4. Contract: `api/app/<pkg>/v1/<pkg>.proto`

Copy `api/app/items/v1/items.proto` and rename: `package app.<pkg>.v1;`, no `go_package` (managed mode in
`backend/buf.gen.yaml` adds it). Then change the fields:

- The file comment names the module directory, says every method acts on the caller's own `<entities>` (another
  user's is `not_found`), and states each limit the service checks and the request path an invalid field is reported
  at (`<entity_snake>.<field>`), as items does. Limits live only in comments here; the service enforces them.
- `message <Entity>`: `string id = 1;`, the fields in the order given (types from step 2, each with a comment giving
  its limit and whether it is required), then `string owner_id`, `google.protobuf.Timestamp create_time`,
  `google.protobuf.Timestamp update_time`, numbered in order. Comment the server-set fields as items does.
- Enums are top-level in the same file, after `<Entity>`, values commented.
- `service <Entity>AppService` with `Create<Entity>`, `Get<Entity>` and `List<Entities>` (both `NO_SIDE_EFFECTS`),
  `Update<Entity>`, `Delete<Entity>`, each commented.
- Requests and responses exactly as items: `Create<Entity>Request { <Entity> <entity_snake> = 1; }`,
  `Get<Entity>Request { string id = 1; }`, `List<Entities>Request { int32 page_size = 1; string page_token = 2; }`
  (copy the comments; keep 20 and 100 unless the user asks), `List<Entities>Response { repeated <Entity> <table> =
  1; string next_page_token = 2; }`, `Update<Entity>Request { <Entity> <entity_snake> = 1;
  google.protobuf.FieldMask update_mask = 2; }` (the comment lists the writable paths), `Delete<Entity>Request {
  string id = 1; }`, `Delete<Entity>Response {}`, and `{ <Entity> <entity_snake> = 1; }` for the other responses.

Adding a package is not a breaking change; `make -C backend api-breaking` confirms (it skips when the base has no
proto).

## Step 5. Generate

```bash
make -C backend gen sqlc
make generate-migration MSG=add_<table>
```

- `gen` runs buf generation into `backend/internal/gen/app/<pkg>/v1/` (`<pkg>.pb.go` and
  `<pkg>v1connect/<pkg>.connect.go`). Run `make -C backend buf-lint` too.
- `sqlc` writes `internal/modules/<pkg>/internal/store/`.
- `generate-migration` (root `Makefile`, needs Docker: Atlas diffs against a throwaway PostgreSQL) writes
  `backend/migrations/<timestamp>_add_<table>.sql` and updates `atlas.sum`. Read the file: it must contain only the
  `CREATE TABLE <table>` and `CREATE INDEX` for this module. If it is wrong, fix `schema.sql`, delete the new
  migration file, restore `backend/migrations/atlas.sum` with `git checkout`, and run the target again. Never edit a
  generated migration; a migration already merged into `main` is never deleted or changed (fix forward with a new
  one).

## Step 6. Handler and wiring: `backend/internal/httpapi/`

Create `backend/internal/httpapi/<pkg>.go` by copying `items.go` and renaming:

- `type <pkg>Handler struct { <pkg> *<pkg>.Service }`, commented with the service it serves.
- `default<Entities>Page = 20`, `max<Entities>Page = 100` (the numbers in the proto's `page_size` comment).
- `var <entity>Writable = []string{...}`: the proto names of the fields a caller may set (every user field).
- `func <pkg>Service(svc *<pkg>.Service) rpc.Service` returning `rpc.Service{Desc:
  <pkg>v1.File_app_<pkg>_v1_<pkg>_proto.Services().ByName("<Entity>AppService"), New: ...}` with
  `<pkg>v1connect.New<Entity>AppServiceHandler(<pkg>Handler{<pkg>: svc}, opts...)`.
- `to<Entity>`: every field, ids with `.String()`, times with `timestamppb.New`, enums and bools per step 2.
- `Create<Entity>`: a nil resource becomes an empty one (the service reports the missing fields); call
  `Create(ctx, ...)`; wrap the error with `rpc.Under("<entity_snake>", err)`.
- `Get<Entity>` and `Delete<Entity>`: `rpc.ParseID("id", req.Msg.GetId())`, then the service; errors are returned as
  they are.
- `List<Entities>`: `rpc.PageRequest(req.Msg.GetPageSize(), req.Msg.GetPageToken(), default<Entities>Page,
  max<Entities>Page)`, the service's `List`, and `NextPageToken: rpc.PageToken(page.Next)`; build the slice with
  `make(..., 0, n)` so an empty page is `[]`.
- `Update<Entity>`: `rpc.ParseID("id", in.GetId())` wrapped in `rpc.Under("<entity_snake>", err)`;
  `paths, err := rpc.UpdatePaths(req.Msg.GetUpdateMask(), in, <entity>Writable...)`; build `<pkg>.Patch` from `paths`
  (required: copy the value into the pointer; optional: `Set<Field> = true` and the generated pointer); wrap the
  service error with `rpc.Under`.

Wire the service:

- `backend/internal/httpapi/server.go`: in `Deps`, add `// <Entities> serves <Entity>AppService. Nil only in tests
  that call none of its methods.` and `<Entities> *<pkg>.Service`; in `connectServices`, add
  `<pkg>Service(deps.<Entities>)` to the services (next to `itemsService(deps.Items)`); add the import.
- `backend/cmd/app/main.go`: add `<Entities>: <pkg>.New(pool),` to the `httpapi.Deps` literal, next to `Items`, and
  the import.
- `backend/internal/httpapi/auth_test.go`: add `<Entities>: <pkg>.New(pool)` to the `Deps` literal in `newAuthAPI`,
  and the import, so the integration tests serve the new methods.

No authorization table to edit: the `AppService` suffix makes `connectEntry` refuse anonymous callers before
decoding, and owner scoping is in the queries.

## Step 7. Integration tests: `backend/internal/httpapi/<pkg>_test.go`

Follow `items_test.go`. The tests use the shared harness (`newAuthAPI`, `a.mustSignUp`, `connectClient`, `baseURL`,
`assertConnectError`, `assertFieldViolation`, `newConnectAPI`, `connectPost`, `decodeWireError`), which runs the real
handler against a migrated PostgreSQL started by testcontainers. Define resource-specific helpers with the resource
name so they never collide with items': `(a *authAPI) <entities>(token)` returning
`<pkg>v1connect.New<Entity>AppServiceClient(connectClient(a.h, token), baseURL, connect.WithHTTPGet())`,
`mustCreate<Entity>`, `get<Entity>`, `mustGet<Entity>`, `update<Entity>` (paths become the `update_mask`; no paths
sends no mask), `mustUpdate<Entity>`, `delete<Entity>`, `list<Entities>`, `mustList<Entities>`,
`assertSame<Entity>` (`proto.Equal`).

Cover behavior, not every field type:

- Create: the returned fields, `owner_id` is the caller, a UUID id, `create_time` equals `update_time`, and Get
  returns the same resource; unset optional fields come back unset; an `id` or `owner_id` the client sends is
  ignored.
- Every method without a session is `unauthenticated` before decoding (copy `TestItemsNeedSession` with the
  `<pkg>v1connect.<Entity>AppService*Procedure` constants).
- Limits for each `string(N)` field: N+1 characters is `assertFieldViolation(t, err, "<entity_snake>.<field>",
  "TOO_LONG")`, `""` and a nil resource are `REQUIRED`, N multi-byte characters are accepted; and the database CHECK
  rejects N+1 characters on a raw `a.pool.Exec` INSERT (give every other NOT NULL column a valid value, `owner_id`
  from `users`). For an enum field, `UNSPECIFIED` is `REQUIRED` and an out-of-range number is `INVALID`.
- List: only the caller's rows, newest first, `next_page_token` empty on the last page; a walk with `page_size: 2`
  over 3 or more rows returns each row once.
- Get: unknown id `not_found`; a non-UUID id is a violation on `id` with `INVALID`; another user's row `not_found`.
- Update: with a mask only the listed fields change (Get after Update); without a mask the set fields change; a
  listed optional field that is unset clears it; an unknown mask path is a violation on `update_mask`; listing a
  server-set field is ignored; `update_time` moves forward and `create_time` does not.
- Delete: succeeds, then Get and Delete are `not_found`; another user's Update and Delete are `not_found` and leave
  the row unchanged.

Paging parameters, malformed tokens and cross-site rejection are shared code, already tested in `items_test.go` and
`connect_test.go`; do not repeat them. `scripts/quality-locks.json` locks the number of backend tests: update its
count in the same commit (`scripts/check_locks.py`).

## Step 8. Frontend

```bash
pnpm -C frontend gen:api
```

This writes `frontend/src/generated/app/<pkg>/v1/<pkg>_pb.ts`, which exports `<Entity>AppService`, the `<Entity>`
type and the message schemas. Building UI is not part of this skill unless the user asked; if they did, put it under
`frontend/src/features/<pkg>/` following `features/auth`:

- reads with connect-query: `useQuery(<Entity>AppService.method.list<Entities>, { pageSize: 20 })` in
  `hooks/use-<entities>.ts`, which starts with `"use client"`;
- writes through `createClient(<Entity>AppService, transport)` (`@/lib/api/transport`) in `api/<entities>-api.ts`,
  called from TanStack Query mutations in the hooks file, which invalidate the list with `createConnectQueryKey`;
- error text with `errorMessage`, field errors with `fieldViolations` and `violationMessage`
  (`@/lib/api/connect-errors`); zod checks only presence, never lengths or enum values.

New reasons are not expected; if you add one to `errors.proto`, add its translation under `errors.fields` or
`errors.reasons` in every `frontend/messages/*.json` (`messages.test.ts` fails otherwise).

## Step 9. Verify

```bash
make -C backend fmt
make -C backend check
pnpm -C frontend gen:api:check
pnpm -C frontend lint
pnpm -C frontend test
```

`make check` needs Docker and runs gofmt, generated-code freshness (`gen-check`, `sqlc-diff`), `buf-lint`,
golangci-lint (the layering rules), the integration tests, govulncheck, schema drift, squawk on the new migration,
migration immutability and the proto breaking-change check (`api-breaking`). Report each command's result to the
user; a failure is fixed, not skipped.
