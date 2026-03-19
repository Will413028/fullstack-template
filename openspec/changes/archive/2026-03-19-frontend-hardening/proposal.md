## Why

The frontend has good architecture (feature-based, i18n, Tanstack Query) but has critical security flaws that prevent authentication from working, zero test coverage, and disabled accessibility checks. These must be fixed before using the template for client projects.

## What Changes

**Critical (Security & Correctness)**
- Fix token storage: switch from localStorage to httpOnly cookies (align middleware + auth hook + api-client)
- Fix CSP: add API URL to connect-src, keep `unsafe-inline` only for styles (Tailwind requirement)
- Re-enable a11y rules in biome.json and fix violations
- Add test framework (Vitest + React Testing Library) with 80%+ coverage target

**High Priority (Completeness)**
- Fix auth types/API to match backend's TokenPair response (access_token + refresh_token)
- Add global error boundary at root layout
- Complete SEO metadata (OG tags, icons, proper sitemap)
- Remove unused code (Zustand useUiStore, stale auth types)

## Capabilities

### New Capabilities
- `auth-token-fix`: Unified cookie-based auth token flow matching backend's JWT + refresh token API
- `csp-fix`: Proper CSP configuration with API URL whitelisting
- `frontend-testing`: Vitest + React Testing Library setup with component and integration tests
- `a11y-compliance`: Re-enabled accessibility rules and ARIA fixes
- `seo-metadata`: Complete metadata, OG tags, proper sitemap
- `error-boundary`: Global error boundary with error logging

### Modified Capabilities

## Impact

- **Auth flow**: Complete rewrite of token storage (localStorage → cookies), auth hooks, and api-client
- **Middleware**: CSP rules updated, token check stays cookie-based (now correct)
- **Testing**: New devDependencies (vitest, @testing-library/react), new test files
- **Biome config**: a11y rules re-enabled
- **Components**: ARIA attributes added to header, dashboard layout
