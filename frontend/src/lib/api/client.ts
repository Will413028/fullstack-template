import createClient, { type Middleware } from "openapi-fetch";
import { appUrl } from "../env";
import type { paths } from "./schema";

// The API is served on the page's own origin under /api (Vercel routes it to
// the API's Caddy, frontend/vercel.json; the next dev rewrite in development),
// so the browser sends the session cookie without CORS. Server-side code has
// no page origin and calls /api on the site's URL, through Vercel's edge too.
//
// Only the headers a caller passes are sent. Next holds no origin secret: the
// edge adds it and writes X-Real-IP, so a server-side call is rate-limited as
// the function's own address and cannot choose another
// (docs/adr/2026-11-03-frontend-on-vercel.md).
function baseUrl(): string {
  if (typeof window !== "undefined") return `${window.location.origin}/api`;
  return `${appUrl()}/api`;
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
