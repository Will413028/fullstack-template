## ADDED Requirements

### Requirement: items.owner_id MUST have database index
The `owner_id` column on the items table SHALL have a database index to support efficient queries by owner.

#### Scenario: Query items by owner uses index
- **WHEN** items are queried filtered by owner_id
- **THEN** the query uses an index scan instead of a sequential scan

#### Scenario: Migration adds index
- **WHEN** the new Alembic migration is applied
- **THEN** an index exists on `items.owner_id`

### Requirement: RefreshTokenRepository MUST extend BaseRepository
`RefreshTokenRepository` SHALL inherit from `BaseRepository[RefreshToken]` to eliminate duplicated CRUD logic while retaining custom query methods.

#### Scenario: Standard CRUD operations work via BaseRepository
- **WHEN** a refresh token is created via RefreshTokenRepository
- **THEN** the BaseRepository.create() method is used (no duplicated add/flush logic)

#### Scenario: Custom queries are preserved
- **WHEN** `get_by_token_hash()` or `revoke_all_for_user()` is called
- **THEN** the custom query logic executes correctly

### Requirement: UserCreateInput MUST validate account length
The `account` field in `UserCreateInput` schema SHALL enforce minimum and maximum length constraints.

#### Scenario: Account too short
- **WHEN** a registration request has account length less than 3 characters
- **THEN** the request is rejected with a 422 validation error

#### Scenario: Account too long
- **WHEN** a registration request has account length greater than 50 characters
- **THEN** the request is rejected with a 422 validation error

#### Scenario: Valid account length
- **WHEN** a registration request has account length between 3 and 50 characters
- **THEN** the validation passes

### Requirement: Unused ListDataResponse MUST be removed
The `ListDataResponse` schema in `core/schemas/base.py` SHALL be removed since it is not used anywhere in the codebase.

#### Scenario: ListDataResponse is removed
- **WHEN** the codebase is searched for ListDataResponse usage
- **THEN** no definition or import of ListDataResponse exists
