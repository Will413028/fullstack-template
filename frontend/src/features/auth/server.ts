import "server-only";

import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { cookies } from "next/headers";
import { UserAppService } from "@/generated/app/users/v1/users_pb";
import { serverTransport } from "@/lib/api/server-transport";
import type { User } from "./api/auth-api";

const currentUser = createClient(UserAppService, serverTransport);

/**
 * Verify the current session from a Server Component by forwarding the request
 * cookies to the backend (GetCurrentUser, a GET that is never cached). Returns
 * the user, or null if unauthenticated.
 * This is the real auth gate — proxy.ts only does a cheap presence redirect.
 *
 * Only unauthenticated means signed out. Anything else (the backend
 * restarting, an internal error) throws to the error boundary: treating it as
 * signed out would send a signed-in user to the login page.
 */
export async function getServerUser(): Promise<User | null> {
  const cookieHeader = (await cookies()).toString();
  if (!cookieHeader) return null;

  let user: User | undefined;
  try {
    ({ user } = await currentUser.getCurrentUser(
      {},
      { headers: { cookie: cookieHeader } },
    ));
  } catch (err) {
    if (ConnectError.from(err).code === Code.Unauthenticated) return null;
    throw err;
  }
  // The contract always sets the user; its absence is the server's bug, not a
  // signed-out user.
  if (!user) {
    throw new ConnectError("GetCurrentUser returned no user", Code.Internal);
  }
  return user;
}
