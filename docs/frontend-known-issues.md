# Frontend Known Issues & Future Work

## Future Work

### Medium Priority
- [ ] Image optimization — use Next.js Image component in shared components. Trigger: the first shared
  component that renders an image.
- [ ] Bundle size analysis — add a bundle analyzer. Trigger: a page's first-load JS grows noticeably in
  `pnpm build` output, or a heavy client dependency is added.

### Low Priority
- [ ] Request timeout configuration in the API client (`src/lib/api/client.ts`). Today nothing bounds how long
  a server-side call waits: the backend's `WriteTimeout` sets a write deadline but does not end a slow handler. Trigger: an operation that may run for seconds, or a
  backend that is reached over a network rather than inside the compose stack.

## Architecture Decisions

### Session Storage: HttpOnly Cookie (implemented)
**Decision:** The session is an opaque token in an HttpOnly cookie set by the backend
(`__Host-session`, or `session` when `COOKIE_SECURE=false`); JS never reads it.
**Why:** HttpOnly cookies cannot be read by XSS. localStorage or JS-readable cookies can be stolen.
**Trade-off:** The frontend and the API must share one origin (Caddy routes `/api`), so the cookie
needs no Domain and the client sends it without `credentials: "include"`.

### CSRF: Fetch Metadata in the Backend (implemented)
**Decision:** The backend wraps every route in Go's `http.CrossOriginProtection`, which rejects
cross-origin state-changing requests (403) using `Sec-Fetch-Site` and `Origin`. No CSRF token.
**Why:** `SameSite=Lax` alone is not a CSRF defense (OWASP CSRF Prevention Cheat Sheet): a sibling
subdomain is the same site, and Lax still sends cookies on top-level cross-site GET navigations.
Fetch Metadata checks are the OWASP-listed defense and need nothing from the frontend.
**Trade-off:** State-changing requests from another origin are rejected; a future cross-origin
client needs its origin allowed in the backend, not a frontend change.

### CSP Strategy
**Decision:** Use nonce-based CSP where possible; `connect-src 'self'`, since the API is on the same origin.
**Why:** `unsafe-inline` defeats CSP. Nonce-based approach is more secure.
**Trade-off:** Next.js with Tailwind requires `unsafe-inline` for styles (Tailwind injects styles). Scripts can use nonces.

### Testing Strategy
**Decision:** Vitest + React Testing Library for unit/integration tests.
**Why:** Vitest is faster than Jest, native ESM support, good Next.js compatibility.
**Trade-off:** No E2E tests; the CI compose job only checks with curl that Caddy serves the API and the pages on one origin.
Trigger for adding Playwright: the first user flow beyond sign-up, sign-in and sign-out, or a regression
that unit tests and the compose job both missed.

