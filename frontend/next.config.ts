import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import { apiUrl, isDevelopment } from "./src/lib/env";

const nextConfig: NextConfig = {
  output: "standalone",
  // Development only: next dev forwards /api to the backend so the browser
  // stays on one origin. In production Caddy routes /api (deploy/Caddyfile)
  // and the build carries no rewrite.
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
