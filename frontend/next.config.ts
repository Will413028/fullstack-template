import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import { apiUrl, isDevelopment } from "./src/lib/env";

const nextConfig: NextConfig = {
  // next dev writes a Next.js agent-rules block into AGENTS.md when it detects
  // an AI agent; the repo keeps its own frontend/AGENTS.md, so leave it alone.
  agentRules: false,
  // Development only: next dev forwards /api to the backend so the browser
  // stays on one origin. On Vercel, frontend/vercel.json routes /api to the
  // API's Caddy with the origin secret, which a rewrite here cannot add, so
  // the build carries no rewrite.
  async rewrites() {
    if (!isDevelopment()) return [];
    return [{ source: "/api/:path*", destination: `${apiUrl()}/api/:path*` }];
  },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
        ],
      },
    ];
  },
};

const withNextIntl = createNextIntlPlugin();

export default withNextIntl(nextConfig);
