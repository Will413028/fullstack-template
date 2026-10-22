FROM node:26.11.1-slim@sha256:193fe51b64e77981119c98c2002c9e32a70e2f006fb4d25068ce0558998917f0 AS base

# Node.js 25 and newer ship without corepack: install the pnpm that package.json's packageManager names, so that field
# stays the one place the version is set. pnpm's own install script links its native binary in place of a
# shebang-less placeholder, which an exec-form CMD cannot run, so it is the one install here that runs scripts. The
# runner stage starts from base and carries no pnpm.
FROM base AS pnpm
WORKDIR /app
COPY package.json ./
RUN npm install --global --no-audit --no-fund \
      "pnpm@$(node -p 'require("./package.json").packageManager.replace(/^pnpm@/, "")')"

FROM pnpm AS deps
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# make dev: next dev with installed dependencies; docker-compose.dev.yml mounts src/, messages/ and public/ over the
# copy so edits reload
FROM deps AS dev
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
CMD ["pnpm", "dev"]

FROM pnpm AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM base AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs \
 && adduser --system --uid 1001 --ingroup nodejs nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

# nextjs:nodejs, by number so a runtime can verify it is not root without reading /etc/passwd
USER 1001:1001
EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["node", "-e", "fetch('http://localhost:3000').then(r => r.ok ? process.exit(0) : process.exit(1)).catch(() => process.exit(1))"]

CMD ["node", "server.js"]
