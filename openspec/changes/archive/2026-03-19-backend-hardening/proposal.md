## Why

The backend has a solid Pragmatic DDD architecture but has security gaps, missing migrations, and operational shortcomings that would cause issues in production. These need to be fixed before using this template for client projects.

## What Changes

**High Priority (Security & Correctness)**
- Generate missing Alembic migration for items table (deployment will crash without it)
- Restrict CORS origins via environment variable (currently defaults to `["*"]`)
- Add SECRET_KEY minimum length validation (HS256 requires at least 32 bytes)
- Fix login timing leak: unify error response for wrong account/password
- Add password complexity validation in UserCreateInput schema

**Medium Priority (Production Readiness)**
- Add refresh token flow (currently only 30-min access token)
- Add rate limiting on auth endpoints (login, register)
- Add comprehensive test suite (auth, items CRUD, edge cases)
- Make logging production-ready (JSON output when MODE != dev)
- Add non-root user to Dockerfile
- Replace python-jose with PyJWT (more maintained, industry standard)

**Low Priority (Polish)**
- Implement graceful shutdown in lifespan (close DB engine)
- Wrap Pydantic validation errors in custom error format
- Cache current user per request to avoid repeated DB lookups

## Capabilities

### New Capabilities
- `security-hardening`: SECRET_KEY validation, password complexity, login timing fix, CORS restriction
- `refresh-token`: Refresh token flow with token rotation
- `rate-limiting`: Per-endpoint rate limiting on sensitive routes
- `test-suite`: Comprehensive async test suite for auth and items modules
- `production-logging`: Environment-aware structured logging (console dev, JSON prod)
- `dockerfile-hardening`: Non-root user, health check in Dockerfile
- `jwt-migration`: Replace python-jose with PyJWT
- `operational-polish`: Graceful shutdown, validation error formatting, user caching

### Modified Capabilities

## Impact

- **Backend code**: Changes to `core/config.py`, `core/security.py`, `core/logging.py`, `auth/service.py`, `auth/schemas.py`, `main.py`, `Dockerfile`
- **New files**: Refresh token model/endpoints, rate limiter middleware, test files
- **Dependencies**: Add `PyJWT`, `slowapi`; remove `python-jose`
- **Database**: New migration for items table, possibly refresh tokens table
- **k8s**: May need to add new env vars to backend secret
