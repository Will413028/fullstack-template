# Frontend Known Issues & Future Work

## Resolved (via frontend-hardening change)

### Critical
- [x] Token storage mismatch: middleware checks cookie, auth hook uses localStorage
- [x] CSP blocks API calls: `connect-src 'self'` doesn't allow cross-origin API
- [x] CSP `unsafe-inline` defeats CSP purpose for script-src/style-src
- [x] Zero test coverage
- [x] All a11y rules disabled in biome.json

### High
- [x] Missing global error boundary
- [x] SEO metadata incomplete (no OG tags, favicon, sitemap)
- [x] Missing register/forgot-password pages (routes defined but no pages)
- [x] Zustand store unused (useUiStore created but sidebar uses useState)
- [x] No CSRF protection on mutations

## Deferred (future changes)

### Medium Priority
- [ ] Theme switching — useUiStore has setTheme but no toggle UI
- [ ] Error logging service integration (Sentry or similar)
- [ ] Form success/loading state feedback (toast notifications)
- [ ] Image optimization — use Next.js Image component in shared components
- [ ] Frontend Dockerfile HEALTHCHECK instruction
- [ ] Bundle size analysis — add webpack-bundle-analyzer
- [ ] RTL support if adding Arabic/Hebrew locales

### Low Priority
- [ ] Offline support — React Query persister
- [ ] Request cancellation / abort controller in api-client
- [ ] Request timeout configuration in api-client
- [ ] E2E tests (Playwright or Cypress)
- [ ] Visual regression testing
- [ ] Storybook for component documentation

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
