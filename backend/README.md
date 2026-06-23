# Backend — FastAPI

Part of the [fullstack template](../README.md). Pragmatic DDD, async SQLAlchemy,
httpOnly-cookie JWT auth. Uses **uv**.

```bash
uv sync                   # install dependencies
make run                  # dev server (http://localhost:8000, /docs for OpenAPI)
make lint                 # ruff format + check
make test                 # pytest (needs a reachable Postgres + DATABASE_URL/SECRET_KEY)
make generate_migration MSG="..."   # alembic autogenerate
make migration            # alembic upgrade head
```

Git hooks are managed by **lefthook** (configured at the repo root) — no
`pre-commit install` step. Linting/formatting run automatically on commit.

Configuration is loaded from `.env` via pydantic-settings (see `.env-example`).
See [`CLAUDE.md`](./CLAUDE.md) for the DDD layering, error-handling rules, and how
to add a module. Normally you run the stack from the repo root (`make dev`).
