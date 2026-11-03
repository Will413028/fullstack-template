# Frontend — Next.js 16

Part of the [template-go-nextjs](../README.md). Uses **pnpm** (not npm/yarn/bun).

```bash
pnpm install        # install dependencies
pnpm dev            # dev server (http://localhost:3000, Turbopack)
pnpm build          # production build (Vercel builds it on deploy)
pnpm lint           # tsc --noEmit + Biome
pnpm format         # Biome format --write
pnpm test           # Vitest
pnpm gen:api        # regenerate src/generated from ../api (Connect)
```

The browser calls the API on its own origin under `/api`. `pnpm dev` on the host
forwards `/api` to `API_URL` (default `http://localhost:8000`, see `.env.example`);
on Vercel, `vercel.json` routes it to the API's Caddy
([ADR](../docs/adr/2026-11-03-frontend-on-vercel.md)).

See [`AGENTS.md`](./AGENTS.md) for architecture, the auth flow, and conventions.
For development run the backend from the repo root with `make dev`, then `pnpm dev` here.
