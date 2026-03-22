## ADDED Requirements

### Requirement: API client MUST auto-refresh on 401 response
The frontend api-client SHALL intercept 401 responses, attempt to refresh the access token using the refresh token, and retry the original request.

#### Scenario: Successful token refresh and retry
- **WHEN** an API request receives a 401 response AND a valid refresh_token cookie exists
- **THEN** the client calls POST /auth/refresh with the refresh token, updates cookies with new tokens, and retries the original request with the new access token

#### Scenario: Refresh token also expired
- **WHEN** an API request receives a 401 response AND the refresh attempt also fails
- **THEN** the client clears all auth cookies and redirects to the login page

#### Scenario: Concurrent requests during refresh
- **WHEN** multiple API requests receive 401 simultaneously
- **THEN** only one refresh request is sent, and all pending requests wait for and use the new token

#### Scenario: Refresh endpoint itself is not retried
- **WHEN** the /auth/refresh endpoint returns 401
- **THEN** the client does NOT attempt to refresh again (no infinite loop), and redirects to login
