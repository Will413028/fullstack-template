// The one module that reads process.env; Biome's noProcessEnv is off for this file only. Each value is a literal
// `process.env.X` read inside a function: Next.js inlines NEXT_PUBLIC_* and NODE_ENV only for literal reads, and
// server-only values such as API_URL are read at call time, not when a client bundle loads this module.

export const isDevelopment = () => process.env.NODE_ENV === "development";

export const appUrl = () =>
  process.env.NEXT_PUBLIC_APP_URL || "https://example.com";

// The backend, for the /api rewrite of next dev only (next.config.ts).
export const apiUrl = () => process.env.API_URL || "http://localhost:8000";

// The site's own origin for server-side API calls, which carry the visitor's
// session cookie. Unset outside development, it is a host that never resolves
// (RFC 6761 .invalid): the call fails instead of sending the cookie to the
// sitemap's placeholder domain.
export const siteOrigin = () =>
  process.env.NEXT_PUBLIC_APP_URL ||
  (isDevelopment() ? "http://localhost:3000" : "https://app-url-unset.invalid");
