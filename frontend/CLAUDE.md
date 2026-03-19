# Frontend — Next.js

Production-ready Next.js frontend with feature-based architecture, i18n, and cookie-based auth.

## Tech Stack

- **Framework:** Next.js 16 (App Router, React 19, TypeScript)
- **Styling:** Tailwind CSS v4 + shadcn/ui
- **Linting:** Biome (not ESLint/Prettier)
- **Package Manager:** pnpm
- **State:** Tanstack React Query (server) + Zustand (client)
- **Forms:** React Hook Form + Zod
- **i18n:** next-intl (en, zh-TW)
- **Animation:** Framer Motion
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
│   ├── shared/           # Reusable (empty-state, motion-wrapper, page-title)
│   └── layout/           # Header, footer
├── lib/                  # Utilities
│   ├── api-client.ts     # Generic HTTP client (cookie-based auth)
│   ├── cookies.ts        # Cookie get/set/remove helpers
│   ├── animations.ts     # Framer Motion presets
│   ├── utils.ts          # cn() Tailwind class merger
│   └── validations.ts    # Shared Zod schemas
├── hooks/                # Custom React hooks (useDebounce, useMediaQuery)
├── providers/            # React Query provider
├── i18n/                 # Routing, request, navigation config
├── store/                # Zustand stores (currently empty)
└── types/                # Global type definitions
```

## Key Conventions

1. **Feature modules** — domain logic lives in `features/<name>/` with api, hooks, components, types
2. **Cookie-based auth** — tokens stored in cookies (not localStorage), middleware reads them for route protection
3. **OAuth2 login** — login sends `application/x-www-form-urlencoded` with username/password fields
4. **Biome, not ESLint** — all linting and formatting via Biome, a11y rules enabled
5. **i18n keys** — all user-facing text in `messages/{locale}.json`, accessed via `useTranslations()`
6. **Server Components by default** — only add `"use client"` when needed (hooks, interactivity)
7. **Suspense for client hooks** — wrap components using `useSearchParams()` in `<Suspense>`

## Auth Flow

```
Login → POST /auth/login (form-encoded) → backend returns TokenPair
     → setCookie("auth_token", access_token)
     → setCookie("refresh_token", refresh_token)
     → redirect to callbackUrl or /overview

Protected route → middleware reads auth_token cookie
               → missing? redirect to /login?callbackUrl=...

API call → api-client reads auth_token cookie → Bearer header

Logout → POST /auth/logout (with refresh_token) → clear cookies
```

## Adding a Feature Module

1. Create `src/features/<name>/` with: `api/`, `hooks/`, `components/`, `types.ts`, `index.ts`
2. API functions use `apiClient` from `@/lib/api-client`
3. Hooks use Tanstack Query (`useQuery`, `useMutation`)
4. Export public API from `index.ts` (barrel file)
5. Add i18n keys to `messages/en.json` and `messages/zh-TW.json`
