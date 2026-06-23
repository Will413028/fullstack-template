# Frontend — Next.js 16

Part of the [fullstack template](../README.md). Uses **pnpm** (not npm/yarn/bun).

```bash
pnpm install        # install dependencies
pnpm dev            # dev server (http://localhost:3000, Turbopack)
pnpm build          # production build (standalone output)
pnpm lint           # tsc --noEmit + Biome
pnpm format         # Biome format --write
pnpm test           # Vitest
```

Set `NEXT_PUBLIC_API_URL` (browser → backend) in `.env`. It is **inlined at build
time**, so changing it for production requires a rebuild, not just a restart.

See [`CLAUDE.md`](./CLAUDE.md) for architecture, the auth flow, and conventions.
Normally you run the whole stack from the repo root with `make dev` / `make up`.
