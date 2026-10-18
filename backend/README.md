# Backend — Go

Part of the [fullstack template](../README.md). Go modular monolith: stdlib `net/http` with an
oapi-codegen strict server, pgx + sqlc, Atlas migrations, opaque session cookies. Needs Go (version
in `go.mod`) and Docker.

```bash
make run                         # serve with backend/.env (copy .env-example; needs a reachable PostgreSQL)
make test                        # integration tests (PostgreSQL 18 via testcontainers)
make check                       # everything CI runs (lint, tests, generated-code, schema and contract checks)
make gen                         # regenerate the HTTP server from api/openapi.yaml
make sqlc                        # regenerate the modules' query code
make migration name=add_things   # generate a migration from the modules' schema.sql
```

The API contract is [`api/openapi.yaml`](api/openapi.yaml); with `MODE=dev` the backend serves it at
`/api/openapi.yaml` and Swagger UI at `/api/docs`. Configuration is read from environment variables
(see `.env-example`); any `KEY` can instead be `KEY_FILE`, a file holding the value.

See [`AGENTS.md`](./AGENTS.md) for the layout, layering, error handling and how to add a module.
Normally you run the whole stack from the repo root (`make dev`).
