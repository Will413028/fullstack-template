## ADDED Requirements

### Requirement: Non-root user in container
The Dockerfile SHALL create and switch to a non-root user before running the application.

#### Scenario: Container runs as non-root
- **WHEN** the container starts
- **THEN** the application process SHALL run as a non-root user

### Requirement: Health check instruction
The Dockerfile SHALL include a HEALTHCHECK instruction.

#### Scenario: Docker health check configured
- **WHEN** the container is running
- **THEN** Docker SHALL report container health based on the `/health` endpoint
