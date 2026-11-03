import { appUrl } from "../env";

// The API is served on the page's own origin under /api (Vercel routes it to the API's Caddy, frontend/vercel.json;
// the next dev rewrite in development), so the browser sends the session cookie without CORS. Server-side code has
// no page origin and calls /api on the site's URL, through Vercel's edge too.
//
// Only the headers a caller passes are sent. Next holds no origin secret: the edge adds it and writes X-Real-IP, so a
// server-side call is rate-limited as the function's own address and cannot choose another
// (docs/adr/2026-11-03-frontend-on-vercel.md). The browser's transport (transport.ts) and the server's
// (server-transport.ts) both use it.
export function apiBaseUrl(): string {
  if (typeof window !== "undefined") return `${window.location.origin}/api`;
  return `${appUrl()}/api`;
}
