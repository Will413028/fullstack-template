## ADDED Requirements

### Requirement: Test framework configured
Vitest and React Testing Library SHALL be configured for component and integration testing.

#### Scenario: Tests run via script
- **WHEN** a developer runs `pnpm test`
- **THEN** Vitest SHALL execute all test files

### Requirement: Component tests exist
Key components SHALL have unit tests.

#### Scenario: Login form tested
- **WHEN** tests run
- **THEN** LoginForm rendering, validation errors, and submit behavior SHALL be tested

### Requirement: API client tests exist
The api-client utility SHALL have tests for request/error handling.

#### Scenario: API client tested
- **WHEN** tests run
- **THEN** successful requests, error responses, and auth header injection SHALL be tested
