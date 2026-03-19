## ADDED Requirements

### Requirement: Accessibility rules enabled in linter
Biome a11y rules SHALL be enabled (not set to "off").

#### Scenario: Linter checks accessibility
- **WHEN** `pnpm lint` runs
- **THEN** accessibility violations SHALL be reported

### Requirement: Interactive elements have ARIA labels
All interactive elements without visible text SHALL have proper ARIA attributes.

#### Scenario: Mobile menu button accessible
- **WHEN** a screen reader encounters the mobile menu button
- **THEN** it SHALL have an `aria-label` describing its action

#### Scenario: Locale dropdown accessible
- **WHEN** a screen reader encounters the locale dropdown
- **THEN** it SHALL have `aria-label`, `aria-expanded`, and `aria-haspopup` attributes
