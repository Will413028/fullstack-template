import createClient, { type Middleware } from "openapi-fetch";
import { apiBaseUrl } from "./base-url";
import type { paths } from "./schema";

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

export const api = createClient<paths>({ baseUrl: apiBaseUrl() });
api.use(redirectOnUnauthenticated);
