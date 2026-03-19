## 1. Auth Token Fix

- [x] 1.1 Update `features/auth/types.ts`: replace AuthResponse with TokenPair matching backend `{ access_token, refresh_token, token_type }`
- [x] 1.2 Create `lib/cookies.ts`: helper functions for get/set/remove cookie (auth_token, refresh_token)
- [x] 1.3 Update `lib/api-client.ts`: read token from cookie instead of localStorage, add `credentials: "include"` to fetch
- [x] 1.4 Update `features/auth/api/auth-api.ts`: login sends form-encoded data (OAuth2 spec), add refresh and logout with refresh_token
- [x] 1.5 Update `features/auth/hooks/use-auth.ts`: store tokens in cookies, add useRefreshToken hook, handle token expiry
- [x] 1.6 Update `features/auth/components/login-form.tsx`: use account (not email) field, show backend error messages
- [x] 1.7 Clean up unused code: remove stale AuthState type, remove localStorage references

## 2. CSP Fix

- [x] 2.1 Update `middleware.ts`: add `NEXT_PUBLIC_API_URL` to CSP connect-src, remove `unsafe-inline` from script-src
- [x] 2.2 Verify frontend builds and API calls work with updated CSP

## 3. Accessibility

- [x] 3.1 Re-enable a11y rules in `biome.json` (remove all "off" entries under a11y)
- [x] 3.2 Fix header.tsx: add aria-label to mobile menu button, aria-expanded + aria-haspopup to locale dropdown
- [x] 3.3 Fix dashboard layout: add aria-label to sidebar toggle, proper landmark roles
- [x] 3.4 Run `pnpm lint` and fix remaining a11y violations

## 4. SEO & Metadata

- [x] 4.1 Update `src/app/layout.tsx`: add OG tags, Twitter cards, icons, theme-color, viewport
- [x] 4.2 Update `src/app/sitemap.ts`: include login page and dashboard overview in sitemap
- [x] 4.3 Add `src/app/[locale]/error.tsx`: global error boundary with retry button and console logging

## 5. Clean Up

- [x] 5.1 Remove unused `src/store/use-ui-store.ts` or integrate it into dashboard layout
- [x] 5.2 Remove `features/auth/validations.ts` loginSchema if replaced, or align with new types

## 6. Testing

- [x] 6.1 Install Vitest + React Testing Library + jsdom (`pnpm add -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom`)
- [x] 6.2 Create `vitest.config.ts` with proper Next.js + path alias setup
- [x] 6.3 Add `"test": "vitest run"` script to package.json
- [x] 6.4 Create `src/lib/__tests__/api-client.test.ts`: test request building, error handling, auth header
- [x] 6.5 Create `src/lib/__tests__/cookies.test.ts`: test cookie get/set/remove helpers
- [x] 6.6 Create `src/features/auth/__tests__/login-form.test.tsx`: test rendering, validation, submit
- [x] 6.7 Run `pnpm test` and verify all tests pass

## 7. Verification

- [x] 7.1 Run `pnpm build` to verify production build succeeds
- [x] 7.2 Run `pnpm lint` to verify no lint errors (including a11y)
