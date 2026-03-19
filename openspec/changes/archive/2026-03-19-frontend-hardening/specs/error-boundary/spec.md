## ADDED Requirements

### Requirement: Global error boundary at locale level
An error.tsx SHALL exist at `src/app/[locale]/error.tsx` to catch all unhandled errors.

#### Scenario: Unhandled error caught
- **WHEN** an unhandled error occurs in any page
- **THEN** the error boundary SHALL display a user-friendly error message with a retry button

#### Scenario: Error logged
- **WHEN** an error is caught by the boundary
- **THEN** the error SHALL be logged to console (production can add external service)
