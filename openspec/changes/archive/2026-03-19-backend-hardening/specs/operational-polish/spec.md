## ADDED Requirements

### Requirement: Graceful shutdown closes database connections
The app lifespan SHALL dispose of the database engine on shutdown.

#### Scenario: Shutdown disposes engine
- **WHEN** the application shuts down
- **THEN** `engine.dispose()` SHALL be called to close all connections

### Requirement: Validation errors use custom format
Pydantic RequestValidationError SHALL be caught and returned in the same JSON format as AppException errors.

#### Scenario: Validation error response format
- **WHEN** a request fails Pydantic validation
- **THEN** the response SHALL have the same `{"detail": "..."}` structure as other errors with status 422
