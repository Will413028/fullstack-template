## ADDED Requirements

### Requirement: No duplicate schema definitions
Each validation schema SHALL exist in exactly one location. The `loginSchema` in `lib/validations.ts` SHALL be removed; the canonical version in `features/auth/validations.ts` SHALL be the single source of truth.

#### Scenario: Only one loginSchema exists
- **WHEN** the codebase is searched for loginSchema definitions
- **THEN** exactly one definition exists in `features/auth/validations.ts`

### Requirement: No unused dependencies in package.json
Dependencies listed in `package.json` MUST be actually used in the codebase.

#### Scenario: zustand is removed
- **WHEN** package.json is inspected
- **THEN** zustand is not listed as a dependency

#### Scenario: Store directory is cleaned up
- **WHEN** the src/store directory is inspected
- **THEN** it is either removed or contains actual store implementations

### Requirement: Dashboard sidebar MUST only show existing routes
The dashboard sidebar SHALL only contain navigation links to routes that have corresponding page implementations.

#### Scenario: No broken navigation links
- **WHEN** a user views the dashboard sidebar
- **THEN** all visible links navigate to existing pages without 404 errors

#### Scenario: Overview link works
- **WHEN** a user clicks the Overview sidebar link
- **THEN** they navigate to the /overview page successfully

### Requirement: Dashboard sidebar labels MUST use i18n
All sidebar navigation labels SHALL use `useTranslations()` from next-intl instead of hardcoded English strings.

#### Scenario: Sidebar shows translated labels
- **WHEN** the locale is zh-TW
- **THEN** sidebar labels display in Traditional Chinese

#### Scenario: Sidebar shows English labels
- **WHEN** the locale is en
- **THEN** sidebar labels display in English
