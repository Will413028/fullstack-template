## ADDED Requirements

### Requirement: PyJWT replaces python-jose
The JWT implementation SHALL use PyJWT instead of python-jose.

#### Scenario: Token creation works with PyJWT
- **WHEN** an access token is created
- **THEN** it SHALL be a valid JWT encoded with PyJWT

#### Scenario: Token validation works with PyJWT
- **WHEN** a JWT is decoded
- **THEN** PyJWT SHALL validate the signature and expiration correctly

#### Scenario: python-jose removed from dependencies
- **WHEN** the dependencies are checked
- **THEN** python-jose SHALL NOT be listed and PyJWT SHALL be present
