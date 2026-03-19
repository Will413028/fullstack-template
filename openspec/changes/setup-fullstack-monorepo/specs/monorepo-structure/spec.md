## ADDED Requirements

### Requirement: Monorepo directory layout
The repository SHALL have a flat structure with `backend/`, `frontend/`, and `k8s/` as top-level directories. Root-level files SHALL include `Makefile`, `CLAUDE.md`, `.claude/` directory, and `README.md`.

#### Scenario: Root directory contains expected structure
- **WHEN** a developer clones the repository
- **THEN** the root directory SHALL contain `backend/`, `frontend/`, `k8s/`, `Makefile`, `CLAUDE.md`, `.claude/`, and `README.md`

#### Scenario: Backend and frontend are independent
- **WHEN** a developer works in `backend/`
- **THEN** the backend SHALL have its own `pyproject.toml`, `Makefile`, `Dockerfile`, and tooling independent of the frontend

#### Scenario: Frontend is independent
- **WHEN** a developer works in `frontend/`
- **THEN** the frontend SHALL have its own `package.json`, `Dockerfile`, and tooling independent of the backend

#### Scenario: k8s manifests are centralized
- **WHEN** a developer needs to modify infrastructure
- **THEN** all Kubernetes manifests SHALL be in the `k8s/` directory, organized by service
