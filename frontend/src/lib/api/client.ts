import createClient, { type Middleware } from "openapi-fetch";
import type { paths } from "./schema";

// The API is served on the page's own origin under /api (Caddy in production,
// the next dev rewrite in development), so the browser sends the session cookie
// without CORS. Server-side code has no page origin and calls the backend
// directly at API_URL.
//
// Only the headers a caller passes are sent. The backend trusts only Caddy
// (TRUSTED_PROXIES), so a server-side call is rate-limited as Next's own
// address whatever X-Forwarded-For it carries.
function baseUrl(): string {
  if (typeof window !== "undefined") return `${window.location.origin}/api`;
  return `${process.env.API_URL || "http://localhost:8000"}/api`;
}

// A 401 means the session is gone. Sessions are opaque and never refreshed, so
// the only remedy is to sign in again. Signing in itself answers 401 for a
// wrong password, which the form shows instead.
const redirectOnUnauthenticated: Middleware = {
  onResponse({ request, response, schemaPath }) {
    if (response.status !== 401 || typeof window === "undefined") return;
    if (request.method === "POST" && schemaPath === "/sessions") return;
    window.location.href = "/login";
  },
};

export const api = createClient<paths>({ baseUrl: baseUrl() });
api.use(redirectOnUnauthenticated);
