## ADDED Requirements

### Requirement: CSP allows API connections
The CSP connect-src directive SHALL include the API URL from environment.

#### Scenario: API calls not blocked by CSP
- **WHEN** the frontend makes API calls to NEXT_PUBLIC_API_URL
- **THEN** CSP SHALL allow the connection

### Requirement: CSP does not use unsafe-inline for scripts
The CSP script-src directive SHALL NOT include `unsafe-inline`.

#### Scenario: Script CSP restrictive
- **WHEN** CSP is applied in production
- **THEN** script-src SHALL be `'self'` without `unsafe-inline`
