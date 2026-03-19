## 1. Security Hardening (High Priority)

- [x] 1.1 Add SECRET_KEY min-length validator (>=32 chars) in `core/config.py` using `@field_validator`
- [x] 1.2 Change CORS_ORIGINS default from `["*"]` to `["http://localhost:3000"]` in `core/config.py`
- [x] 1.3 Fix login timing leak: always run `verify_password` even when user not found, unify error to "Invalid credentials" in `auth/service.py`
- [x] 1.4 Add password complexity validation (min 8 chars, uppercase, lowercase, digit) to `auth/schemas.py` UserCreateInput
- [x] 1.5 Generate Alembic migration for items table (`alembic revision --autogenerate`)

## 2. JWT Migration

- [x] 2.1 Replace `python-jose` with `PyJWT` in `pyproject.toml` dependencies
- [x] 2.2 Update `core/security.py`: change `from jose import jwt` to `import jwt`, adjust `jwt.decode()` call to use `algorithms=[...]` parameter
- [x] 2.3 Update `auth/dependencies.py`: change `JWTError` import from jose to `jwt.exceptions.PyJWTError`
- [x] 2.4 Run `uv lock` to update lockfile

## 3. Refresh Token

- [x] 3.1 Create `auth/models.py` RefreshToken model (token hash, user_id FK, expires_at, revoked, created_at)
- [x] 3.2 Update `auth/schemas.py`: add `TokenPair` schema with access_token + refresh_token, add `RefreshTokenInput`
- [x] 3.3 Update `core/security.py`: add `create_refresh_token()` function (random bytes, sha256 hash for storage)
- [x] 3.4 Update `auth/repository.py`: add refresh token CRUD methods (create, get by token hash, revoke)
- [x] 3.5 Update `auth/service.py`: login returns token pair, add `refresh()` and `logout()` methods
- [x] 3.6 Update `auth/router.py`: add POST `/auth/refresh` and POST `/auth/logout` endpoints
- [x] 3.7 Generate Alembic migration for refresh_tokens table

## 4. Rate Limiting

- [x] 4.1 Add `slowapi` to `pyproject.toml` dependencies
- [x] 4.2 Create rate limiter instance and add to app in `main.py`
- [x] 4.3 Apply `@limiter.limit("5/minute")` to `/auth/login` and `/auth/register` in `auth/router.py`

## 5. Production Logging

- [x] 5.1 Update `core/logging.py`: use JSONRenderer + stdlib logging when MODE != "dev", keep ConsoleRenderer for dev

## 6. Dockerfile Hardening

- [x] 6.1 Add non-root user (`useradd --system appuser`) and `USER appuser` before CMD in `backend/Dockerfile`
- [x] 6.2 Add `HEALTHCHECK` instruction pointing to `/health` endpoint

## 7. Operational Polish

- [x] 7.1 Add `await engine.dispose()` in lifespan shutdown in `main.py`
- [x] 7.2 Register `RequestValidationError` handler in `exception_handlers.py` to return consistent JSON format

## 8. Test Suite

- [x] 8.1 Update `tests/conftest.py`: add fixtures for test user creation, auth tokens, authenticated client
- [x] 8.2 Create `tests/test_auth.py`: register, login, duplicate registration, invalid credentials, refresh token, protected endpoint access
- [x] 8.3 Create `tests/test_items.py`: CRUD lifecycle, ownership enforcement, pagination
- [ ] 8.4 Run full test suite and verify all tests pass (requires DB connection from host)

## 9. Verification

- [x] 9.1 Run `make build` from root to verify Docker image builds with all changes
- [x] 9.2 Run `make deploy` and verify backend starts and passes health check
