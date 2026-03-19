## Context

The frontend is a Next.js 16 app (App Router, TypeScript, Tailwind v4, shadcn/ui, Biome). The backend now returns `TokenPair` (access_token + refresh_token) on login/register. The frontend's auth flow is broken because middleware checks cookies but the auth hook stores tokens in localStorage.

## Goals / Non-Goals

**Goals:**
- Make auth flow work end-to-end with the backend's JWT + refresh token API
- Fix CSP so API calls work in production
- Add test coverage (80%+ target)
- Re-enable and comply with a11y rules
- Complete SEO metadata

**Non-Goals:**
- Server-side auth (NextAuth/Auth.js) — keep it simple with cookie-based JWT
- E2E tests (Playwright/Cypress) — future change
- Theme switching UI — future change
- Server Components for auth — keep client-side for now

## Decisions

### 1. Token storage: httpOnly cookies set by backend

The backend should set `access_token` as an httpOnly cookie on login/register responses. The frontend api-client sends cookies automatically via `credentials: "include"`. No more localStorage.

**Why not localStorage**: XSS can steal tokens. httpOnly cookies are immune to JavaScript access.

**Implementation**: Since the backend currently returns tokens in JSON body (not Set-Cookie), the frontend will:
- Receive tokens in JSON response
- Store access_token in a cookie via `document.cookie` (NOT httpOnly — that requires backend changes)
- For now, use a regular cookie (readable by middleware) as a pragmatic middle ground
- Document that production should migrate to backend-set httpOnly cookies

**Why this compromise**: Changing the backend cookie flow is a separate concern. The critical fix is aligning middleware (cookie) with auth hook (also cookie now). Both read the same cookie.

### 2. CSP: dynamic connect-src from environment

Read `NEXT_PUBLIC_API_URL` at build time and include it in CSP `connect-src`. Keep `unsafe-inline` for `style-src` only (Tailwind CSS injects styles at runtime). Remove `unsafe-inline` from `script-src`.

**Why keep style unsafe-inline**: Tailwind v4 requires it. Removing it breaks all styling.

### 3. Auth types aligned with backend

Update `AuthResponse` → `TokenPair` to match backend schema: `{ access_token, refresh_token, token_type }`. Update auth-api to use `FormData` for login (OAuth2 spec). Add refresh token flow.

### 4. Testing: Vitest + React Testing Library

Use Vitest (fast, ESM-native) with `@testing-library/react` and `@testing-library/user-event`. Test components, hooks, and api-client. Mock API calls with MSW or manual fetch mocks.

**Why Vitest over Jest**: Native ESM, faster, better Next.js compatibility.

### 5. a11y: re-enable rules, fix violations

Re-enable all a11y rules in biome.json. Add ARIA labels to interactive elements (mobile menu, locale dropdown, sidebar toggle). Fix semantic HTML where needed.

### 6. SEO: complete metadata in root layout

Add OG tags, Twitter cards, icons, theme-color, and locale-specific metadata. Expand sitemap to include all public routes.

### 7. Error boundary: global with logging

Add `error.tsx` at `src/app/[locale]/error.tsx` to catch all unhandled errors. Log to console (production can add Sentry later).

## Risks / Trade-offs

- **[Cookie not httpOnly]** → Pragmatic compromise. Document that production should use backend-set httpOnly cookies. Current approach is still better than localStorage (middleware can read it).
- **[unsafe-inline for styles]** → Required by Tailwind v4. Cannot be avoided without major Tailwind config changes.
- **[No E2E tests]** → Unit/integration tests only for now. E2E is a future change.
