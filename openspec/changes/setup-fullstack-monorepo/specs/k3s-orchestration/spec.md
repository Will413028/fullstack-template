## ADDED Requirements

### Requirement: k8s manifest directory structure
A `k8s/` directory at the monorepo root SHALL contain Kubernetes manifests organized by service: `postgres/`, `backend/`, `frontend/`, plus a shared `namespace.yaml`.

#### Scenario: k8s directory exists with expected structure
- **WHEN** the setup is complete
- **THEN** `k8s/` SHALL contain `namespace.yaml`, `postgres/`, `backend/`, and `frontend/` subdirectories each with their respective manifests

### Requirement: Namespace isolation
All Kubernetes resources SHALL be created in a dedicated project namespace defined in `k8s/namespace.yaml`.

#### Scenario: Resources deployed to project namespace
- **WHEN** `make deploy` is run
- **THEN** all pods, services, secrets, and volumes SHALL exist within the project namespace

#### Scenario: Clean teardown via namespace deletion
- **WHEN** `make teardown` is run
- **THEN** deleting the namespace SHALL remove all project resources

### Requirement: PostgreSQL deployment with persistent storage
The PostgreSQL service SHALL use a Deployment with a PersistentVolumeClaim for data persistence and a Secret for credentials.

#### Scenario: PostgreSQL pod running with persistent data
- **WHEN** `make deploy` is run
- **THEN** a PostgreSQL pod SHALL be running with a PVC-backed volume mounted at `/var/lib/postgresql/data`

#### Scenario: PostgreSQL credentials from Secret
- **WHEN** the PostgreSQL pod starts
- **THEN** it SHALL read `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` from a Kubernetes Secret

#### Scenario: Data survives pod restart
- **WHEN** the PostgreSQL pod is deleted and recreated
- **THEN** data SHALL be preserved via the PersistentVolumeClaim

### Requirement: Backend deployment with migration init container
The backend Deployment SHALL use an init container to run Alembic migrations before the main FastAPI container starts.

#### Scenario: Migrations run before app starts
- **WHEN** the backend pod starts
- **THEN** an init container SHALL run `alembic upgrade head` to completion before the FastAPI container starts

#### Scenario: Backend connects to PostgreSQL
- **WHEN** the backend container is running
- **THEN** it SHALL connect to PostgreSQL using `DATABASE_URL` from a Kubernetes Secret

#### Scenario: Backend accessible within cluster
- **WHEN** all services are running
- **THEN** the backend SHALL be accessible via a ClusterIP Service on port 8000

### Requirement: Frontend deployment with configurable API URL
The frontend Deployment SHALL use a ConfigMap to configure the backend API URL.

#### Scenario: Frontend uses API URL from ConfigMap
- **WHEN** the frontend pod starts
- **THEN** it SHALL read `NEXT_PUBLIC_API_URL` from a ConfigMap

#### Scenario: Frontend accessible externally
- **WHEN** all services are running
- **THEN** the frontend SHALL be accessible via a NodePort Service

### Requirement: Root Makefile for k3s operations
A root `Makefile` SHALL provide targets for common k3s operations.

#### Scenario: Build images
- **WHEN** a developer runs `make build`
- **THEN** Docker images for backend and frontend SHALL be built and importable into k3s

#### Scenario: Deploy all services
- **WHEN** a developer runs `make deploy`
- **THEN** all Kubernetes manifests in `k8s/` SHALL be applied in the correct order

#### Scenario: Teardown all services
- **WHEN** a developer runs `make teardown`
- **THEN** the project namespace and all its resources SHALL be deleted

#### Scenario: View logs
- **WHEN** a developer runs `make logs`
- **THEN** logs from all pods in the project namespace SHALL be displayed

### Requirement: Environment configuration template
A `.env-example` file SHALL be provided at the monorepo root with all required environment variables for k3s deployment.

#### Scenario: .env-example documents all variables
- **WHEN** a developer clones the repository
- **THEN** `.env-example` SHALL list all required variables: database credentials, secret key, API URL, and any k3s-specific settings
