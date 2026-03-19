## ADDED Requirements

### Requirement: Auth tokens stored in cookies
Login and register SHALL store access_token in a cookie readable by both middleware and client code.

#### Scenario: Login stores token in cookie
- **WHEN** a user logs in successfully
- **THEN** the access_token SHALL be stored in a cookie named `auth_token`
- **AND** the refresh_token SHALL be stored in a cookie named `refresh_token`

#### Scenario: Middleware reads auth cookie
- **WHEN** a user visits a protected route
- **THEN** middleware SHALL read the `auth_token` cookie to determine auth status

#### Scenario: API client sends auth cookie
- **WHEN** the api-client makes authenticated requests
- **THEN** it SHALL read the `auth_token` cookie and send it as a Bearer token

### Requirement: Auth types match backend TokenPair
The frontend auth types SHALL match the backend's response format.

#### Scenario: Login response parsed correctly
- **WHEN** the backend returns `{ access_token, refresh_token, token_type }`
- **THEN** the frontend SHALL parse and store both tokens

### Requirement: Login uses OAuth2 form data
The login API call SHALL use `application/x-www-form-urlencoded` format with `username` and `password` fields.

#### Scenario: Login sends form data
- **WHEN** a user submits the login form
- **THEN** the API call SHALL send form-encoded data (not JSON) matching OAuth2 spec

### Requirement: Logout clears tokens
Logout SHALL remove auth cookies and call the backend logout endpoint with the refresh token.

#### Scenario: Logout clears state
- **WHEN** a user logs out
- **THEN** both `auth_token` and `refresh_token` cookies SHALL be removed
- **AND** the backend `/auth/logout` endpoint SHALL be called
