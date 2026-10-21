---
name: add-crud
description: Generate a complete user-owned CRUD resource in the Go backend (schema.sql, sqlc queries, service, httpapi handlers, OpenAPI paths, integration tests) and regenerate the frontend API types
user_invocable: true
---

# /add-crud <entity> --fields "<field_definitions>"

Generate a complete CRUD resource owned by the signed-in user: a new module under `backend/internal/modules/`, its
table, its sqlc queries, the HTTP operations in `backend/api/openapi.yaml` and `backend/internal/httpapi/`, a
generated migration, integration tests, and regenerated frontend types. The reference implementation is the `items`
module; when this file and `items` disagree, stop and tell the user instead of guessing.

## Rules the generated code must follow

- Modules never import `net/http`, `internal/httpapi`, `cmd/`, or another module (depguard in `backend/.golangci.yml`
  and `backend/internal/archtest` fail the build otherwise). Handlers live only in `backend/internal/httpapi`.
- SQL is written only in `queries/*.sql` and called only through the sqlc-generated `internal/store` package. Never
  edit generated files (`internal/store/*`, `backend/internal/httpapi/gen/api.gen.go`,
  `frontend/src/lib/api/schema.d.ts`).
- DDL is written only in the module's `schema.sql` (the desired state). Migrations are generated with
  `make -C backend migration name=<x>` (Atlas diff); never write or edit a migration by hand. `backend/atlas.hcl`
  globs `internal/modules/*/schema.sql` and loads auth first, so a new `schema.sql` is picked up without editing it.
- Primary key `id uuid PRIMARY KEY DEFAULT uuidv7()`; text columns are `text` with a CHECK that matches the
  contract's `minLength`/`maxLength`; enums are `text` with `CHECK (col IN (...))`; times are `timestamptz`; every
  table has `created_at` and `updated_at timestamptz NOT NULL DEFAULT now()`, and every UPDATE query sets
  `updated_at = now()`.
- The resource is owned: `owner_id uuid NOT NULL REFERENCES users (id)` (users is the only table another module may
  reference) and every query has `owner_id = @owner_id`. Another user's row is a 404, exactly like a missing one.
- Lengths, formats and enums are declared only in `backend/api/openapi.yaml`. The request validator enforces them
  before the handler runs; do not re-check them in Go handlers, services, or frontend zod schemas.
- Responses: the resource itself, no envelope. Create is 201 with `Location: /api/<path>/<id>` (a URL that answers
  GET), delete is 204 with no body, list is keyset-paginated (`limit`, `cursor`, `next_cursor`, newest first, no
  total), PATCH accepts only `application/merge-patch+json` (RFC 7396).

If the user asks for something outside these rules (a resource not owned by a user, a reference to another module's
table, a `decimal`/money field, a list sorted by something other than creation time), stop and ask how they want it
designed; this skill does not generate it.

## 1. Parse the arguments

- `<entity>`: the resource name, singular or plural, in snake_case (`note`, `notes`, `todo_list`). Derive the names
  below; if the plural is irregular or ambiguous, ask.

  | Placeholder | Rule | `notes` | `todo_list` |
  |---|---|---|---|
  | `<pkg>` | module directory and Go package: plural, lowercase, no separators | `notes` | `todolists` |
  | `<table>` | table name and JSON array property: plural snake_case | `notes` | `todo_lists` |
  | `<path>` | URL segment: plural kebab-case | `notes` | `todo-lists` |
  | `<Entity>` / `<Entities>` | Go and OpenAPI names: PascalCase singular / plural | `Note` / `Notes` | `TodoList` / `TodoLists` |
  | `<entity>` / `<entities>` | camelCase singular / plural (operationIds, test helpers) | `note` / `notes` | `todoList` / `todoLists` |

  Stop if `backend/internal/modules/<pkg>` already exists.

- `--fields`: comma-separated `name:type`, each optionally followed by `required` or `optional` (a trailing `?` on
  the type also means optional). Fields are required unless marked optional. Names are snake_case; `id`, `owner_id`,
  `created_at` and `updated_at` are reserved (every table gets them). Example:
  `"title:string(200) required, body:text optional, done:bool, due:date?, status:enum(open|closed)"`.

## 2. Field type mapping

| User type | OpenAPI schema | PostgreSQL column + CHECK | Go: sqlc store and service | Go: oapi-codegen (`gen`) |
|---|---|---|---|---|
| `string(N)` | named component `<Entity><Field>`: `type: string`, `minLength: 1`, `maxLength: N`, `description: 1 to N characters (code points).` | `text` + `CHECK (char_length(f) BETWEEN 1 AND N)` | `string` | `string` (alias `gen.<Entity><Field> = string`) |
| `text` | `type: string` (no limits; the 1 MiB body limit caps it) | `text` | `string` | `string` |
| `bigint` | `type: integer`, `format: int64` | `bigint` | `int64` | `int64` |
| `float` | `type: number`, `format: double` | `double precision` | `float64` | `float64` |
| `bool` | `type: boolean` | `boolean` | `bool` | `bool` |
| `date` | `type: string`, `format: date` | `date` | `time.Time` (sqlc.yaml override) | `openapi_types.Date`: build with `openapi_types.Date{Time: t}`, read with `d.Time` |
| `datetime` | `type: string`, `format: date-time` | `timestamptz` | `time.Time` | `time.Time` (send `.UTC()`) |
| `enum(a\|b\|c)` | named component `<Entity><Field>`: `type: string`, `enum: [a, b, c]` | `text` + `CHECK (f IN ('a', 'b', 'c'))` | `string` | `gen.<Entity><Field>`: convert with `gen.<Entity><Field>(s)` and `string(v)` |

Required fields are `NOT NULL`. Optional fields are nullable columns (no `NOT NULL`; a CHECK passes on NULL), `*T` in
the store and service, and `nullable: true` in the contract. Where an optional field uses a named component, write
it as `allOf: [{$ref: "#/components/schemas/<Entity><Field>"}]` plus `nullable: true`.

The Go types the generators produce, by position (`T` is the column above):

| | Required field | Optional field |
|---|---|---|
| sqlc row, service struct, Create parameter | `T` | `*T` |
| `gen.<Entity>` (response) | `T` | `nullable.Nullable[T]` |
| `gen.Create<Entity>Request` | `T` | `nullable.Nullable[T]` |
| `gen.<Entity>Patch` | `*T` | `nullable.Nullable[T]` |

sqlc names follow Go initialisms (`ID`, `OwnerID`); oapi-codegen does not (`Id`, `OwnerId`).

## 3. Module: `backend/internal/modules/<pkg>/`

### `schema.sql`

Columns in this order: `id`, the fields in the order given, `owner_id`, `created_at`, `updated_at`. Align like
`items/schema.sql`.

```sql
-- <pkg> module: desired state of its table. Atlas generates migrations from this file (make migration); never edit
-- applied migrations.

-- owner_id -> users.id is the only foreign key to another module; the <pkg> queries never read users. No cascade:
-- deleting a user who still owns <table> fails instead of silently removing them.
CREATE TABLE <table> (
  id          uuid        PRIMARY KEY DEFAULT uuidv7(),
  -- one line per field, from the mapping table, e.g.
  -- title    text        NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  -- body     text,
  owner_id    uuid        NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Keyset pagination: WHERE owner_id = $1 AND id < $cursor ORDER BY id DESC.
CREATE INDEX <table>_owner_id_id_idx ON <table> (owner_id, id DESC);
```

### `sqlc.yaml`

```yaml
# <pkg> module: the schema includes auth's schema.sql only so sqlc resolves <table>.owner_id -> users.id; the
# queries must not read users. Generated code goes to internal/store (Go's internal keeps other modules out); never
# edit it by hand. Run make sqlc from backend/.
version: "2"
sql:
  - engine: postgresql
    schema: [../auth/schema.sql, schema.sql]
    queries: queries
    gen:
      go:
        package: store
        out: internal/store
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
        # The auth tables are in the schema only for the foreign key; do not generate structs for them.
        omit_unused_structs: true
        # Without these, sqlc emits pgtype.Date and pgtype.Timestamptz for date and nullable timestamptz.
        overrides:
          - db_type: uuid
            go_type: github.com/google/uuid.UUID
          - db_type: uuid
            nullable: true
            go_type:
              import: github.com/google/uuid
              type: UUID
              pointer: true
          - db_type: timestamptz
            go_type: time.Time
          - db_type: timestamptz
            nullable: true
            go_type:
              type: time.Time
              pointer: true
          - db_type: date
            go_type: time.Time
          - db_type: date
            nullable: true
            go_type:
              type: time.Time
              pointer: true
```

### `queries/<table>.sql`

`<cols>` is the full column list in table order: `id, <fields...>, owner_id, created_at, updated_at`.

```sql
-- Every query is scoped to the owner: another user's row behaves as if it did not exist. Updates and deletes report
-- the rows they touched, so "not yours" is a 404 rather than a silent success.

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

-- Update<Entity> applies a merge patch: a null <required> leaves it unchanged (the contract forbids <required>:
-- null), and set_<optional> says whether <optional> was sent, so null can clear it.
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

- `type <Entity> struct`: exactly the fields of the generated `store.<Entity>`, same names, types and order
  (`ID`, the fields, `OwnerID`, `CreatedAt`, `UpdatedAt`), so `<Entity>(row)` converts a row. Check against
  `internal/store/models.go` after `make sqlc`.
- `type Page struct { <Entities> []<Entity>; NextCursor string }`.
- `type Patch struct`: per required field `<Field> *T`; per optional field `Set<Field> bool` followed by
  `<Field> *T`. `empty()` returns true when every required pointer is nil and every `Set<Field>` is false. Document
  the merge-patch meaning on the type as items does.
- `Create(ctx, <field> T / *T ...)`: one parameter per field, in field order, passed to `store.Create<Entity>Params`
  together with `OwnerID`.
- `Get`, `List`, `Update`, `Delete`: as in items, with `errNotFound = apperr.New(apperr.NotFound, "", "<entity>
  not found")` and error prefixes `"<pkg>: create: %w"` and so on. `List` uses `httpx.After`, `httpx.PageRows` and
  `httpx.TrimPage` exactly as items does; `Update` returns `s.Get(ctx, id)` for an empty patch.
- `owner(ctx)` reads `authctx.From(ctx)` and returns an error (not an `apperr`) when no user is present: the contract
  requires a session for these operations, so a missing user is a wiring bug.

## 4. Contract: `backend/api/openapi.yaml`

Copy the `/items` and `/items/{id}` path items and the `Item`, `ItemPage`, `CreateItemRequest`, `ItemPatch` schemas,
then rename and change the fields:

- Paths `/<path>` (`post` operationId `create<Entity>`, `get` operationId `list<Entities>`) and `/<path>/{id}`
  (`get` `get<Entity>`, `patch` `update<Entity>`, `delete` `delete<Entity>`), `tags: [<pkg>]`, summaries and
  descriptions rewritten for the resource. Keep every response code and `$ref` that items lists for the same method
  (Problem responses under `components/responses`, `Location` header on the 201, `Limit`/`Cursor` parameters on the
  list, `Id` path parameter, `application/merge-patch+json` on the patch). Do not add `security`: the document-level
  `sessionCookie` requirement applies, which is what makes the validator return 401 without a session.
- Place the new paths after the last existing path, and the new schemas after the last resource's schemas (before
  `Problem`). Use the same order and indentation as items.
- `<Entity>`: `required` lists every property (`id`, the fields, `owner_id`, `created_at`, `updated_at`); optional
  fields are `nullable: true`; no length or enum limits here except a `$ref` to an enum component.
- `<Entity>Page`: `required: [<table>, next_cursor]`, `<table>` an array of `<Entity>`, `next_cursor` as in
  `ItemPage`.
- `Create<Entity>Request`: `additionalProperties: false`, `required` lists the required fields, optional fields are
  `nullable: true`; string(N) and enum fields `$ref` their component.
- `<Entity>Patch`: `additionalProperties: false`, no `required`; required fields without `nullable` (so `null` is
  rejected with `null_not_allowed`), optional fields `nullable: true`. Describe the merge-patch semantics in the
  `patch` operation's description as items does.
- Components `<Entity><Field>` for each `string(N)` and `enum` field, placed right after `<Entity>Patch` (as
  `ItemTitle` follows `ItemPatch`), so limits are written once.

Adding operations and schemas is not a breaking change; `make check` runs `api-breaking` to confirm.

## 5. Generate

From the repository root (Docker must be running: Atlas diffs against a throwaway PostgreSQL):

```bash
make -C backend gen sqlc migration name=add_<table>
```

- `gen` rewrites `internal/httpapi/gen/api.gen.go`; the build fails until step 6 implements the new methods.
- `sqlc` writes `internal/modules/<pkg>/internal/store/`.
- `migration` writes `backend/migrations/<timestamp>_add_<table>.sql` and updates `atlas.sum`. Read the file: it must
  contain only the `CREATE TABLE <table>` and `CREATE INDEX` for this module. If it is wrong, fix `schema.sql`,
  delete the new migration file, restore `backend/migrations/atlas.sum` with `git checkout`, and run the migration
  target again. Never edit a generated migration; a migration already merged into `main` is never deleted or
  changed (fix forward with a new one).

## 6. Handlers and wiring: `backend/internal/httpapi/`

Create `backend/internal/httpapi/<pkg>.go` modelled on `items.go` (`<module>` is the module path on the first line of
`backend/go.mod`; `make rename` changes it):

```go
package httpapi

import (
	"context"

	"<module>/internal/httpapi/gen"
	"<module>/internal/modules/<pkg>"
)

// The <pkg> operations (/<path>). The <pkg> service scopes every query to the signed-in user.

func to<Entity>(v <pkg>.<Entity>) gen.<Entity> {
	return gen.<Entity>{
		Id: v.ID, /* fields */ OwnerId: v.OwnerID, CreatedAt: v.CreatedAt.UTC(), UpdatedAt: v.UpdatedAt.UTC(),
	}
}

func (s *apiServer) Create<Entity>(ctx context.Context, req gen.Create<Entity>RequestObject) (gen.Create<Entity>ResponseObject, error) {
	v, err := s.<pkg>.Create(ctx /* , fields */)
	if err != nil {
		return nil, err
	}
	location := "/api/<path>/" + v.ID.String()
	return gen.Create<Entity>201JSONResponse{Body: to<Entity>(v), Headers: gen.Create<Entity>201ResponseHeaders{Location: &location}}, nil
}

func (s *apiServer) List<Entities>(ctx context.Context, req gen.List<Entities>RequestObject) (gen.List<Entities>ResponseObject, error) {
	page, err := s.<pkg>.List(ctx, pageSize(req.Params.Limit), req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	resp := gen.List<Entities>200JSONResponse{<Entities>: make([]gen.<Entity>, 0, len(page.<Entities>)), NextCursor: nextCursor(page.NextCursor)}
	for _, v := range page.<Entities> {
		resp.<Entities> = append(resp.<Entities>, to<Entity>(v))
	}
	return resp, nil
}

func (s *apiServer) Get<Entity>(ctx context.Context, req gen.Get<Entity>RequestObject) (gen.Get<Entity>ResponseObject, error) {
	v, err := s.<pkg>.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.Get<Entity>200JSONResponse(to<Entity>(v)), nil
}

func (s *apiServer) Update<Entity>(ctx context.Context, req gen.Update<Entity>RequestObject) (gen.Update<Entity>ResponseObject, error) {
	p := <pkg>.Patch{ /* fields */ }
	v, err := s.<pkg>.Update(ctx, req.Id, p)
	if err != nil {
		return nil, err
	}
	return gen.Update<Entity>200JSONResponse(to<Entity>(v)), nil
}

func (s *apiServer) Delete<Entity>(ctx context.Context, req gen.Delete<Entity>RequestObject) (gen.Delete<Entity>ResponseObject, error) {
	if err := s.<pkg>.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.Delete<Entity>204Response{}, nil
}
```

Field conversions, using the helpers `optional`, `fromNullable`, `pageSize` and `nextCursor` (defined in
`internal/httpapi/items.go`; reuse them, never redefine them in the same package):

| Where | Required field | Optional field |
|---|---|---|
| `to<Entity>` | `F: v.F` | `F: optional(v.F)` |
| `Create(...)` argument | `req.Body.F` | `fromNullable(req.Body.F)` |
| `Patch{...}` | `F: req.Body.F` | `SetF: req.Body.F.IsSpecified(), F: fromNullable(req.Body.F)` |

Type-specific additions: `datetime` values get `.UTC()` on output; `date` and `enum` fields convert between the
store type and the `gen` type as in the mapping table (for optional or patch values, convert the pointer: `nil`
stays `nil`). Put any conversion helper you need in `<pkg>.go`, named after the resource
(`to<Entity><Field>`), so two resources never define the same function.

If `rg -n 'func (optional|fromNullable|pageSize|nextCursor)\b' backend/internal/httpapi` finds nothing (the items
module was removed), move those four functions and the `defaultPageSize` constant from the git history of
`internal/httpapi/items.go` into `internal/httpapi/pagination.go` first.

Wire the service:

- `backend/internal/httpapi/server.go`: in `Deps`, add `// <Entities> serves the <pkg> operations. Nil only in
  tests that call none of them.` and `<Entities> *<pkg>.Service`; in `apiServer`, add `<pkg> *<pkg>.Service`; in
  `NewHandler`, add `<pkg>: deps.<Entities>` to the `&apiServer{...}` literal; add the import.
- `backend/cmd/app/main.go`: add `<Entities>: <pkg>.New(pool),` to the `httpapi.Deps` literal in `serve`, next to
  `Items`, and the import.
- `backend/internal/httpapi/auth_test.go`: add `<Entities>: <pkg>.New(pool)` to the `Deps` literal in
  `newAuthAPI`, and the import, so the integration tests serve the new operations.

- `backend/internal/httpapi/authz.go`: add each new operation to `operationAccess` as `accessUser` (owner scoping is
  in the queries); `accessAdmin` only for an operation restricted to admins. The server does not start while an
  operation is missing from the table.

## 7. Integration tests: `backend/internal/httpapi/<pkg>_test.go`

Follow `items_test.go`. The tests use the shared harness (`newAuthAPI`, `a.mustSignUp`, `a.do`, `assertProblem`,
`assertFieldError`, `mustJSON`), which runs the real handler against a migrated PostgreSQL started by
testcontainers and checks on every 201 that `Location` answers GET. Define resource-specific helpers with the
resource name so they never collide with items': `create<Entity>`, `mustCreate<Entity>`, `get<Entity>`,
`patch<Entity>` (Content-Type `application/merge-patch+json`), `delete<Entity>`, `list<Entities>`,
`decode<Entity>`, and `<entity>Fields` (decode to `map[string]any` to see `null` versus missing).

Cover behavior, not every field type:

- Create: 201, body fields, `owner_id`, `Location` equals `/api/<path>/<id>`; optional fields omitted come back as
  `null`; unauthenticated is 401 `/problems/unauthenticated`.
- Limits for each `string(N)` field: N+1 characters is 422 `too_long` at `/<field>`, `""` is `too_short`, a missing
  required field is `required`, N multi-byte characters are accepted; and the database CHECK rejects N+1
  characters on a raw `a.pool.Exec` INSERT (give every other NOT NULL column a valid value, `owner_id` from
  `users`). For an enum field, an unknown value is 422 at `/<field>`.
- List: only the caller's rows, newest first, `next_cursor` null on the last page, `[]` (not null) when empty; a
  walk with `?limit=2` over 3 or more rows returns each row once.
- Get: 200; unknown id 404 `/problems/not-found`; another user's row 404.
- Update: changes persist (GET after PATCH); omitted fields stay; `null` clears each optional field; `null` on a
  required field is 422 `null_not_allowed`; `{}` changes nothing; `application/json` is 415
  `/problems/unsupported-media-type`; `updated_at` moves forward and `created_at` does not.
- Delete: 204 with an empty body, then GET and DELETE are 404; another user's PATCH and DELETE are 404 and leave the
  row unchanged.

Cross-site rejection, the default page size and malformed cursors are shared middleware, already tested in
`items_test.go`; do not repeat them.

## 8. Frontend types

```bash
pnpm -C frontend gen:api
```

This regenerates `frontend/src/lib/api/schema.d.ts`; the types are then `components["schemas"]["<Entity>"]` and the
paths are available on the typed `api` client from `@/lib/api/client` (wrap calls with `unwrap` from
`@/lib/api/errors`, as `src/features/auth/api/auth-api.ts` does). Building UI is not part of this skill unless the
user asked; if they did, put it under `frontend/src/features/<pkg>/` and check only presence in zod, never lengths
or enums. New problem codes are not expected; if you add one to the contract, add its translation under
`problems.codes` in every `frontend/messages/*.json`.

## 9. Verify

```bash
make -C backend fmt
make -C backend check
pnpm -C frontend lint
pnpm -C frontend test
```

`make check` needs Docker and runs gofmt, generated-code freshness (`gen-check`, `sqlc-diff`), golangci-lint (the
layering rules), the integration tests, govulncheck, schema drift, squawk on the new migration, migration
immutability and the OpenAPI breaking-change check. `api-breaking` compares with `origin/main`; if it prints that
the base has no `api/openapi.yaml`, also run `make -C backend api-breaking BASE=HEAD` before committing so the check
compares with the last commit. Report each command's result to the user; a failure is fixed, not skipped.
