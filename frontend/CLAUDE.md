# Frontend — Next.js

Production-ready Next.js frontend with feature-based architecture, i18n, and cookie-based auth.

## Tech Stack

- **Framework:** Next.js 16 (App Router, React 19, TypeScript)
- **Styling:** Tailwind CSS v4 + shadcn/ui
- **Linting:** Biome (not ESLint/Prettier)
- **Package Manager:** pnpm
- **State:** Tanstack React Query (server) + Zustand (client UI state)
- **Forms:** React Hook Form + Zod
- **i18n:** next-intl (en, zh-TW)
- **Auth:** httpOnly cookies (set by the backend) + server-side session guard
- **Testing:** Vitest + React Testing Library

## Quick Reference

```bash
pnpm dev              # Dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript + Biome check
pnpm format           # Biome format
pnpm test             # Run Vitest
pnpm test:watch       # Watch mode
```

## Architecture

```
src/
├── app/[locale]/         # File-based routing with i18n
│   ├── (auth)/           # Auth pages (login)
│   ├── (dashboard)/      # Protected pages (overview)
│   └── (marketing)/      # Public pages
├── features/             # Feature modules
│   └── auth/             # Auth: api, hooks, components, types, validations
├── components/
│   ├── ui/               # shadcn/ui components
│   └── layout/           # Header, footer, dashboard-shell
├── lib/                  # Utilities
│   ├── api-client.ts     # Generic HTTP client (credentials:"include")
│   ├── server-auth.ts    # getServerUser() — server-side session check
│   └── utils.ts          # cn() Tailwind class merger
├── hooks/                # Custom React hooks (useDebounce)
├── providers/            # React Query provider
├── i18n/                 # Routing, request, navigation config
├── store/                # Zustand stores (use-ui-store: sidebar state)
└── proxy.ts              # Presence redirect + CSP headers (advisory, not the gate)
```

## Key Conventions

1. **Feature modules** — domain logic lives in `features/<name>/` with api, hooks, components, types
2. **httpOnly cookie auth** — the backend sets httpOnly+Secure cookies; JS never reads tokens. `proxy.ts` does a cheap presence redirect; the real gate is `getServerUser()` in the `(dashboard)` server-component layout
3. **OAuth2 login** — login sends `application/x-www-form-urlencoded` with username/password fields
4. **Biome, not ESLint** — all linting and formatting via Biome, a11y rules enabled
5. **i18n keys** — all user-facing text in `messages/{locale}.json`, accessed via `useTranslations()`
6. **Server Components by default** — only add `"use client"` when needed (hooks, interactivity)
7. **Suspense for client hooks** — wrap components using `useSearchParams()` in `<Suspense>`

## Auth Flow (httpOnly cookies)

```
Login → POST /auth/login (form-encoded) → backend sets httpOnly access_token +
        refresh_token cookies and returns the user → redirect to callbackUrl/overview

API call → api-client sends credentials:"include"; the browser attaches the
           cookies automatically (JS never touches tokens)

401 → api-client POSTs /auth/refresh (the cookie carries the refresh token) → retry;
      if refresh fails → redirect to /login

Protected route → proxy.ts redirects on a missing cookie (UX only); the (dashboard)
                 layout calls getServerUser() server-side and redirects on an
                 invalid session (the real gate)

Logout → POST /auth/logout → backend clears the cookies
```

## Adding a Feature Module

1. Create `src/features/<name>/` with: `api/`, `hooks/`, `components/`, `types.ts`, `index.ts`
2. API functions use `apiClient` from `@/lib/api-client`
3. Hooks use Tanstack Query (`useQuery`, `useMutation`)
4. Export public API from `index.ts` (barrel file)
5. Add i18n keys to `messages/en.json` and `messages/zh-TW.json`
