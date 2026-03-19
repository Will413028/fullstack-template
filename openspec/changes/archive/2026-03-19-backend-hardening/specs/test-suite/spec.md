## ADDED Requirements

### Requirement: Auth endpoint tests
Tests SHALL cover register, login, token refresh, and protected endpoint access.

#### Scenario: Register and login flow
- **WHEN** a new user registers and then logs in
- **THEN** both operations SHALL succeed and return valid tokens

#### Scenario: Duplicate registration rejected
- **WHEN** a user tries to register with an existing account
- **THEN** a 409 error SHALL be returned

#### Scenario: Invalid credentials rejected
- **WHEN** login is attempted with wrong credentials
- **THEN** a 401 error SHALL be returned

#### Scenario: Protected endpoint requires auth
- **WHEN** a protected endpoint is called without a token
- **THEN** a 401 error SHALL be returned

### Requirement: Items CRUD tests
Tests SHALL cover create, read, update, delete operations for items.

#### Scenario: CRUD lifecycle
- **WHEN** an authenticated user creates, reads, updates, and deletes an item
- **THEN** each operation SHALL succeed with correct responses

#### Scenario: Ownership enforcement
- **WHEN** a user tries to modify another user's item
- **THEN** a 403 error SHALL be returned

### Requirement: Pagination tests
Tests SHALL verify pagination parameters and response format.

#### Scenario: Paginated list
- **WHEN** items are requested with page and size parameters
- **THEN** the response SHALL include correct pagination metadata
