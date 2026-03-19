## ADDED Requirements

### Requirement: Backend files migrated into backend/ subdirectory
All source code, tests, migrations, configuration, and tooling from `fastapi-template` SHALL be copied into the `backend/` directory preserving the existing directory structure.

#### Scenario: Source code preserved
- **WHEN** the migration is complete
- **THEN** `backend/src/` SHALL contain the full FastAPI application with `main.py`, `core/`, `auth/`, `items/`, and `health/` modules

#### Scenario: Tests preserved
- **WHEN** the migration is complete
- **THEN** `backend/tests/` SHALL contain all existing test files

#### Scenario: Alembic migrations preserved
- **WHEN** the migration is complete
- **THEN** `backend/alembic/` and `backend/alembic.ini` SHALL exist with all existing migration versions

### Requirement: Backend Dockerfile adjusted for monorepo context
The backend `Dockerfile` SHALL work correctly when built with `docker-compose.yml` from the monorepo root using `./backend` as build context.

#### Scenario: Docker build succeeds from monorepo root
- **WHEN** `docker compose build backend` is run from the monorepo root
- **THEN** the build SHALL complete successfully using `backend/Dockerfile`

### Requirement: Backend Makefile works from backend/ directory
The `Makefile` SHALL work when `make` commands are run from the `backend/` directory.

#### Scenario: Make run works
- **WHEN** a developer runs `make run` from `backend/`
- **THEN** the FastAPI dev server SHALL start successfully

#### Scenario: Make test works
- **WHEN** a developer runs `make test` from `backend/`
- **THEN** pytest SHALL execute all tests

### Requirement: Backend docker-compose example removed
The backend-specific `docker-compose.yml-example` SHALL NOT be included in `backend/`, as it is superseded by the root-level `docker-compose.yml`.

#### Scenario: No duplicate compose file
- **WHEN** the migration is complete
- **THEN** `backend/docker-compose.yml-example` SHALL NOT exist

### Requirement: Backend CLAUDE.md preserved
The backend's `CLAUDE.md` SHALL be kept at `backend/CLAUDE.md` for backend-specific Claude Code context.

#### Scenario: Backend CLAUDE.md exists
- **WHEN** the migration is complete
- **THEN** `backend/CLAUDE.md` SHALL exist with backend architecture and conventions
