import { api } from "@/lib/api/client";
import { unwrap } from "@/lib/api/errors";
import type { components } from "@/lib/api/schema";

export type User = components["schemas"]["User"];
export type Credentials = components["schemas"]["CreateSessionRequest"];

// The session token lives only in the HttpOnly cookie the backend sets; these
// calls return the user and never see the token.
export const authApi = {
  async login(credentials: Credentials): Promise<User> {
    const session = await unwrap(api.POST("/sessions", { body: credentials }));
    return session.user;
  },

  register(credentials: Credentials): Promise<User> {
    return unwrap(api.POST("/users", { body: credentials }));
  },

  async logout(): Promise<void> {
    await unwrap(api.DELETE("/sessions/current"));
  },

  getMe(): Promise<User> {
    return unwrap(api.GET("/users/me"));
  },
};
