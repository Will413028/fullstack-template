## Context

The FastAPI backend uses Pragmatic DDD with async SQLAlchemy. It works but has security gaps (CORS, SECRET_KEY, login timing, password rules), missing infrastructure (items migration, refresh tokens, rate limiting, tests), and operational issues (logging, Dockerfile, JWT library). All changes are within `backend/`.

## Goals / Non-Goals

**Goals:**
- Fix all security vulnerabilities without changing the DDD architecture
- Add refresh token flow compatible with the existing auth module pattern
- Add rate limiting on auth endpoints
- Build a test suite covering auth and items CRUD
- Make logging, Dockerfile, and error handling production-ready
- Replace python-jose with PyJWT

**Non-Goals:**
- Changing the DDD architecture or module structure
- Adding new business modules
- Frontend integration (frontend already has api-client ready)
- Adding Redis or external caching (keep it simple)

## Decisions

### 1. Password validation: Pydantic field_validator on UserCreateInput

Add min 8 chars, max 100, at least one uppercase, one lowercase, one digit. Implemented as a Pydantic `field_validator` in `auth/schemas.py`.

**Why not a separate validator lib**: Pydantic is already there. Keep it simple.

### 2. Login timing fix: always hash-check even when user not found

When user is not found, still run `verify_password` against a dummy hash to prevent timing-based account enumeration. Return a single generic "Invalid credentials" error for both cases.

**Why**: Constant-time response prevents attackers from distinguishing "user exists" from "user doesn't exist".

### 3. Refresh tokens: stored in DB, rotated on use

Add `RefreshToken` model with `token`, `user_id`, `expires_at`, `revoked`. On login, issue both access + refresh tokens. On refresh, revoke old token and issue new pair.

**Why DB-stored over stateless**: Enables revocation, logout-all, and token rotation. The extra DB query per refresh is acceptable since it happens infrequently.

### 4. Rate limiting: slowapi on auth endpoints only

Use `slowapi` (built on `limits` library) with in-memory backend. Apply `5/minute` on `/auth/login` and `/auth/register`. No global rate limit.

**Why slowapi**: Mature, FastAPI-compatible, minimal setup. In-memory is fine for single-instance; production can swap to Redis backend.

### 5. JWT library: PyJWT replaces python-jose

Drop-in replacement. Change `from jose import jwt` to `import jwt` (PyJWT). API is nearly identical: `jwt.encode()`, `jwt.decode()`.

**Why PyJWT**: More actively maintained, larger community, recommended by most FastAPI tutorials. python-jose hasn't been updated in years.

### 6. Logging: environment-aware configuration

When `MODE == "dev"`: ConsoleRenderer + PrintLoggerFactory (current behavior).
When `MODE != "dev"`: JSONRenderer + stdlib logging integration for structured JSON output.

**Why**: Dev needs readable console output. Production needs machine-parseable JSON for log aggregation (ELK, Loki, etc.).

### 7. Test strategy: async fixtures with test database

Use `pytest-asyncio` with `httpx.AsyncClient`. Create fixtures for: authenticated user, admin user, test items. Test against the real app (not mocks) using the same DB.

**Why no mocks**: The user's existing `conftest.py` already uses real app testing. Mocks can diverge from reality.

### 8. Dockerfile: add non-root user and HEALTHCHECK

Add `RUN useradd --system appuser` and `USER appuser` before CMD. Add `HEALTHCHECK` instruction pointing to `/health`.

### 9. Graceful shutdown: close engine in lifespan

In `main.py` lifespan, call `await engine.dispose()` on shutdown to properly close connection pool.

### 10. Validation error formatting: add RequestValidationError handler

Register a handler for `RequestValidationError` that returns the same JSON structure as AppException errors, for consistency.

### 11. User caching: skip for now

Per-request DB lookup is acceptable overhead. Adding caching introduces complexity (invalidation, stale data) without meaningful benefit at template scale. Documented as future optimization.

## Risks / Trade-offs

- **[slowapi in-memory storage]** → Rate limit state lost on restart. Acceptable for template; production should use Redis backend. Documented in CLAUDE.md.
- **[Refresh tokens in same DB]** → Additional table and queries. Acceptable trade-off for revocation support.
- **[PyJWT migration]** → Minor API differences. `jwt.decode()` requires `algorithms` parameter (already passed). Low risk.
- **[Test DB same as dev DB]** → Tests may interfere with dev data. Mitigated by using transactions that roll back.
