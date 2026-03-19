## ADDED Requirements

### Requirement: Rate limiting on auth endpoints
The `/auth/login` and `/auth/register` endpoints SHALL be rate-limited to 5 requests per minute per IP.

#### Scenario: Within rate limit
- **WHEN** fewer than 5 requests are made in a minute from the same IP
- **THEN** all requests SHALL be processed normally

#### Scenario: Rate limit exceeded
- **WHEN** more than 5 requests are made in a minute from the same IP
- **THEN** a 429 Too Many Requests response SHALL be returned with a Retry-After header
