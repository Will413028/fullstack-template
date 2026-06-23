import "server-only";

import { cookies } from "next/headers";
import type { UserResponse } from "@/features/auth/types";

// Server-side base URL: inside Docker the frontend container reaches the backend
// by service name (http://backend:8000), which differs from the browser-facing
// NEXT_PUBLIC_API_URL. Falls back to it (and localhost) for local dev.
function serverApiUrl(): string {
  return (
    process.env.API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    "http://localhost:8000"
  );
}

/**
 * Verify the current session from a Server Component by forwarding the request
 * cookies to the backend. Returns the user, or null if unauthenticated.
 * This is the real auth gate — middleware only does a cheap presence redirect.
 */
export async function getServerUser(): Promise<UserResponse | null> {
  const cookieHeader = (await cookies()).toString();
  if (!cookieHeader) return null;

  try {
    const res = await fetch(new URL("/auth/me", serverApiUrl()).toString(), {
      headers: { cookie: cookieHeader },
      cache: "no-store",
    });
    if (!res.ok) return null;
    const json = (await res.json()) as { data: UserResponse };
    return json.data;
  } catch {
    return null;
  }
}
