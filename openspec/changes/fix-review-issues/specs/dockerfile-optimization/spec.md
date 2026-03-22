## ADDED Requirements

### Requirement: Backend Dockerfile MUST use multi-stage build
The backend Dockerfile SHALL use a multi-stage build to separate build-time dependencies (gcc) from the runtime image.

#### Scenario: Runtime image does not contain gcc
- **WHEN** the final Docker image is inspected
- **THEN** gcc is not installed in the image

#### Scenario: Runtime image contains only runtime dependencies
- **WHEN** the final Docker image is inspected
- **THEN** only libpq (runtime library for asyncpg) and Python runtime are present

### Requirement: Dockerfile CMD MUST NOT run migrations
The backend Dockerfile CMD SHALL only start the FastAPI server. Migrations SHALL be handled exclusively by the K8s init container.

#### Scenario: Docker CMD only starts FastAPI
- **WHEN** the Dockerfile CMD is inspected
- **THEN** it runs only the FastAPI server command, without any alembic commands

#### Scenario: K8s init container handles migrations
- **WHEN** the backend pod starts in K8s
- **THEN** the init container runs alembic upgrade head before the main container starts

### Requirement: Makefile deploy MUST depend on build
The `deploy` Makefile target SHALL declare `build` as a prerequisite to prevent deploying stale images.

#### Scenario: Deploy triggers build first
- **WHEN** `make deploy` is executed
- **THEN** Docker images are built before K8s manifests are applied
