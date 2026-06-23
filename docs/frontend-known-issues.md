# Frontend Known Issues & Future Work

## Future Work

### Medium Priority
- [ ] Image optimization — use Next.js Image component in shared components
- [ ] Bundle size analysis — add webpack-bundle-analyzer

### Low Priority
- [ ] Request timeout configuration in api-client

## Architecture Decisions

### Token Storage: httpOnly Cookies (implemented)
**Decision:** Auth tokens are httpOnly+Secure cookies set by the backend; JS never reads them.
**Why:** httpOnly cookies are immune to XSS. localStorage/JS-readable cookies can be stolen via XSS.
**Trade-off:** Requires backend to set cookies and `credentials:"include"` on the client.
`SameSite=Lax` blocks cross-site POST cookie sending, covering CSRF for state-changing
requests; add a CSRF token only if you later need cross-site flows (`SameSite=None`).

### CSP Strategy
**Decision:** Use nonce-based CSP where possible, allow `connect-src` for API URL.
**Why:** `unsafe-inline` defeats CSP. Nonce-based approach is more secure.
**Trade-off:** Next.js with Tailwind requires `unsafe-inline` for styles (Tailwind injects styles). Scripts can use nonces.

### Testing Strategy
**Decision:** Vitest + React Testing Library for unit/integration tests.
**Why:** Vitest is faster than Jest, native ESM support, good Next.js compatibility.
**Trade-off:** No E2E tests in initial setup (deferred to future change).

