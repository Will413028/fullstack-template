## ADDED Requirements

### Requirement: Login returns access and refresh tokens
The login endpoint SHALL return both an access token and a refresh token.

#### Scenario: Successful login
- **WHEN** valid credentials are provided
- **THEN** the response SHALL include `access_token`, `refresh_token`, and `token_type`

### Requirement: Refresh token endpoint
A POST `/auth/refresh` endpoint SHALL accept a refresh token and return a new token pair.

#### Scenario: Valid refresh token
- **WHEN** a valid, non-expired, non-revoked refresh token is provided
- **THEN** a new access_token and refresh_token SHALL be returned and the old refresh token SHALL be revoked

#### Scenario: Expired refresh token
- **WHEN** an expired refresh token is provided
- **THEN** a 401 error SHALL be returned

#### Scenario: Revoked refresh token
- **WHEN** a previously revoked refresh token is provided
- **THEN** a 401 error SHALL be returned

### Requirement: Logout revokes refresh token
The logout endpoint SHALL revoke the user's refresh token.

#### Scenario: Logout with refresh token
- **WHEN** a user calls POST `/auth/logout` with a refresh token
- **THEN** that refresh token SHALL be marked as revoked

### Requirement: Refresh token stored in database
Refresh tokens SHALL be stored in a `refresh_tokens` table with user_id, token hash, expiration, and revoked flag.

#### Scenario: Refresh token persisted on login
- **WHEN** a user logs in
- **THEN** a refresh token record SHALL be created in the database
