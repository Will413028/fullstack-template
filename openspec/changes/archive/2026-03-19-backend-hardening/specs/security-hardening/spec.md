## ADDED Requirements

### Requirement: SECRET_KEY minimum length validation
The Settings class SHALL validate that SECRET_KEY is at least 32 characters long at startup.

#### Scenario: Valid SECRET_KEY accepted
- **WHEN** SECRET_KEY is 32+ characters
- **THEN** the application SHALL start normally

#### Scenario: Short SECRET_KEY rejected
- **WHEN** SECRET_KEY is fewer than 32 characters
- **THEN** the application SHALL fail to start with a clear validation error

### Requirement: CORS origins from environment
The CORS_ORIGINS setting SHALL default to `["http://localhost:3000"]` instead of `["*"]`.

#### Scenario: Default CORS restricts origins
- **WHEN** CORS_ORIGINS is not set in environment
- **THEN** only `http://localhost:3000` SHALL be allowed

#### Scenario: Custom CORS origins from env
- **WHEN** CORS_ORIGINS is set to a comma-separated list
- **THEN** those origins SHALL be used for CORS

### Requirement: Login timing-safe error response
The login endpoint SHALL return the same error message and take similar time regardless of whether the account exists.

#### Scenario: Wrong password
- **WHEN** a valid account is provided with a wrong password
- **THEN** the response SHALL be 401 with message "Invalid credentials"

#### Scenario: Non-existent account
- **WHEN** a non-existent account is provided
- **THEN** the response SHALL be 401 with message "Invalid credentials" and SHALL still perform a password hash comparison

#### Scenario: Disabled account
- **WHEN** a disabled account is provided with correct password
- **THEN** the response SHALL be 401 with message "Invalid credentials"

### Requirement: Password complexity validation
The UserCreateInput schema SHALL enforce password complexity rules.

#### Scenario: Valid password accepted
- **WHEN** password has 8+ chars, at least one uppercase, one lowercase, one digit
- **THEN** registration SHALL proceed

#### Scenario: Short password rejected
- **WHEN** password is fewer than 8 characters
- **THEN** a 422 validation error SHALL be returned

#### Scenario: Password missing uppercase rejected
- **WHEN** password has no uppercase letter
- **THEN** a 422 validation error SHALL be returned

### Requirement: Items table migration exists
An Alembic migration SHALL exist for the items table.

#### Scenario: Fresh database setup
- **WHEN** `alembic upgrade head` is run on an empty database
- **THEN** both `users` and `items` tables SHALL be created
