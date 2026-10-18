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
- **API:** types generated from `backend/api/openapi.yaml` (openapi-typescript) + openapi-fetch, same-origin `/api`
- **Auth:** opaque session in an HttpOnly cookie (set by the backend) + server-side session guard
- **Testing:** Vitest + React Testing Library

## Quick Reference

```bash
pnpm dev              # Dev server (Turbopack)
pnpm build            # Production build
pnpm lint             # TypeScript + Biome check
pnpm format           # Biome format
pnpm test             # Run Vitest
pnpm test:watch       # Watch mode
pnpm gen:api          # Regenerate src/lib/api/schema.d.ts after changing the contract
```

## Architecture

```
src/
├── app/[locale]/         # File-based routing with i18n
│   ├── (auth)/           # Auth pages (login, register)
│   ├── (dashboard)/      # Protected pages (overview)
│   └── (marketing)/      # Public pages
├── features/             # Feature modules
│   └── auth/             # Auth: api, hooks, components, validations,
│                         #   server.ts (getServerUser() — server-side session check)
├── components/
│   ├── ui/               # shadcn/ui components
│   └── layout/           # Header, footer, dashboard-shell
├── lib/                  # Utilities
│   ├── api/              # schema.d.ts (generated), client.ts (openapi-fetch),
│   │                     #   errors.ts (problem → ApiError and translated messages)
│   └── utils.ts          # cn() Tailwind class merger
├── hooks/                # Custom React hooks (useDebounce)
├── providers/            # React Query provider
├── i18n/                 # Routing, request, navigation config
├── store/                # Zustand stores (use-ui-store: sidebar state)
└── proxy.ts              # Presence redirect + CSP headers (advisory, not the gate)
```

## Key Conventions

1. **Feature modules** — domain logic lives in `features/<name>/` with api, hooks, components; types come from the generated `components["schemas"]`, never hand-written
2. **The contract is the single source** — `backend/api/openapi.yaml` defines shapes, lengths, formats and enums; run `pnpm gen:api` after changing it (CI fails on a stale `schema.d.ts`). Forms check presence only and show the API's field errors
3. **Errors are RFC 9457 problems** — show messages by `type` and `code` from `messages/*.json` (`problems.*`, via `lib/api/errors.ts`); never show or parse `detail`. A new `type` or `code` in the contract needs a message in every locale, or `pnpm lint` fails
4. **Biome, not ESLint** — all linting and formatting via Biome, a11y rules enabled
5. **i18n keys** — all user-facing text in `messages/{locale}.json`, accessed via `useTranslations()`
6. **Server Components by default** — only add `"use client"` when needed (hooks, interactivity)
7. **Suspense for client hooks** — wrap components using `useSearchParams()` in `<Suspense>`

## Auth Flow (opaque session cookie)

```
Same origin → the browser calls /api on the page's own origin; Caddy (Docker,
              make dev and make up) or the pnpm dev rewrite (host) forwards it
              to the backend. No CORS, no API URL in the bundle.

Sign up → POST /api/users (JSON) → 201 with the User; the backend sets the
          session cookie (__Host-session, or session with COOKIE_SECURE=false)
Sign in → POST /api/sessions (JSON) → 201 with a Session; the user is .user

API call → the browser sends the cookie on its own (same origin); JS never
           reads it

401 → the client sends the user to /login. There is no refresh: the session is
      one opaque token, valid until its idle or absolute limit. A 401 from
      signing in is a wrong password and stays in the form.

Protected route → proxy.ts redirects only when no session cookie is present
                 (UX only; it never sends /login on to /overview, because the
                 cookie may be stale). The (dashboard) layout calls
                 getServerUser() server-side: a 401 redirects to /login, any
                 other failure throws to the error boundary (the real gate)

Sign out → DELETE /api/sessions/current → 204; the backend clears the cookie
```

## Adding a Feature Module

1. Create `src/features/<name>/` with: `api/`, `hooks/`, `components/`, `index.ts`
2. API functions call `api` from `@/lib/api/client` and pass the result to `unwrap()` from `@/lib/api/errors`
3. Hooks use Tanstack Query (`useQuery`, `useMutation`)
4. Export public API from `index.ts` (barrel file); server-only code goes in `server.ts`, imported as `@/features/<name>/server`, never re-exported from the barrel (client components import it)
5. `lib/` holds generic utilities and never imports from `features/`
6. Add i18n keys to `messages/en.json` and `messages/zh-TW.json`
