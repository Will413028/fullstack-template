import { Code, ConnectError, createClient } from "@connectrpc/connect";
import {
  type CreateSessionRequest,
  SessionPublicService,
} from "@/generated/app/sessions/v1/sessions_pb";
import {
  type User,
  UserAppService,
  UserPublicService,
} from "@/generated/app/users/v1/users_pb";
import { transport } from "@/lib/api/transport";

export type { User };
export type Credentials = Pick<CreateSessionRequest, "account" | "password">;

const sessions = createClient(SessionPublicService, transport);
const users = createClient(UserPublicService, transport);
const currentUser = createClient(UserAppService, transport);

// The contract always sets the user on these responses; a response without one is the server's bug, not a
// signed-out user.
function userOf(user: User | undefined, method: string): User {
  if (!user)
    throw new ConnectError(`${method} returned no user`, Code.Internal);
  return user;
}

// The session token lives only in the HttpOnly cookie the backend sets; these
// calls return the user and never see the token.
export const authApi = {
  async login(credentials: Credentials): Promise<User> {
    const { session } = await sessions.createSession(credentials);
    return userOf(session?.user, "CreateSession");
  },

  async register(credentials: Credentials): Promise<User> {
    const { user } = await users.createUser(credentials);
    return userOf(user, "CreateUser");
  },

  async logout(): Promise<void> {
    await sessions.deleteCurrentSession({});
  },

  async getMe(): Promise<User> {
    const { user } = await currentUser.getCurrentUser({});
    return userOf(user, "GetCurrentUser");
  },

  // Deleting the account needs a sign-in from the last few minutes
  // (REAUTHENTICATION_REQUIRED otherwise), so it signs in with the password
  // first; the new session is the one the delete then ends.
  async deleteAccount(credentials: Credentials): Promise<void> {
    await sessions.createSession(credentials);
    await currentUser.deleteCurrentUser({});
  },
};
