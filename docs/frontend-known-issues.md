# Frontend Known Issues & Future Work

## Future Work

### Medium Priority
- [ ] Image optimization — use Next.js Image component in shared components
- [ ] Bundle size analysis — add webpack-bundle-analyzer

### Low Priority
- [ ] Request timeout configuration in api-client

## Architecture Decisions

### Token Storage: httpOnly Cookies
**Decision:** Use httpOnly cookies for auth tokens instead of localStorage.
**Why:** httpOnly cookies are immune to XSS. localStorage tokens can be stolen via XSS.
**Trade-off:** Requires backend to set cookies; CSRF protection needed for mutations.

### CSP Strategy
**Decision:** Use nonce-based CSP where possible, allow `connect-src` for API URL.
**Why:** `unsafe-inline` defeats CSP. Nonce-based approach is more secure.
**Trade-off:** Next.js with Tailwind requires `unsafe-inline` for styles (Tailwind injects styles). Scripts can use nonces.

### Testing Strategy
**Decision:** Vitest + React Testing Library for unit/integration tests.
**Why:** Vitest is faster than Jest, native ESM support, good Next.js compatibility.
**Trade-off:** No E2E tests in initial setup (deferred to future change).

