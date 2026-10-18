import "server-only";

import { cookies } from "next/headers";
import { api } from "@/lib/api/client";
import { unwrap } from "@/lib/api/errors";
import type { User } from "./api/auth-api";

/**
 * Verify the current session from a Server Component by forwarding the request
 * cookies to the backend. Returns the user, or null if unauthenticated.
 * This is the real auth gate — proxy.ts only does a cheap presence redirect.
 *
 * Only a 401 means signed out. Anything else (the backend restarting, a 5xx)
 * throws to the error boundary: treating it as signed out would send a
 * signed-in user to the login page.
 */
export async function getServerUser(): Promise<User | null> {
  const cookieHeader = (await cookies()).toString();
  if (!cookieHeader) return null;

  const result = await api.GET("/users/me", {
    headers: { cookie: cookieHeader },
    cache: "no-store",
  });
  if (result.response.status === 401) return null;
  return unwrap(Promise.resolve(result));
}
