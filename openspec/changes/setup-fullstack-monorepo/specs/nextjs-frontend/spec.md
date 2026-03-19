## ADDED Requirements

### Requirement: Next.js project scaffolded with App Router
The frontend SHALL be a Next.js project created with App Router, TypeScript, and Tailwind CSS in the `frontend/` directory.

#### Scenario: Frontend initialised
- **WHEN** the setup is complete
- **THEN** `frontend/` SHALL contain a valid Next.js project with `package.json`, `tsconfig.json`, `next.config.*`, and `app/` directory

#### Scenario: Dev server starts
- **WHEN** a developer runs `npm run dev` from `frontend/`
- **THEN** the Next.js development server SHALL start and serve the default page

### Requirement: Frontend Dockerfile for production build
The frontend SHALL include a `Dockerfile` that produces a production-ready Next.js container.

#### Scenario: Docker build succeeds
- **WHEN** `docker compose build frontend` is run from the monorepo root
- **THEN** the build SHALL complete successfully and produce a runnable container

#### Scenario: Container serves the application
- **WHEN** the frontend container is running
- **THEN** it SHALL serve the Next.js application on port 3000

### Requirement: Frontend environment configuration
The frontend SHALL support environment variables for API base URL configuration to connect to the backend.

#### Scenario: Backend API URL configurable
- **WHEN** the frontend is started with `NEXT_PUBLIC_API_URL` environment variable
- **THEN** the frontend SHALL use that URL as the backend API base URL
