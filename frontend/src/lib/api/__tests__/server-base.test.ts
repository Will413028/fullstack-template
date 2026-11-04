import { createClient } from "@connectrpc/connect";
import { expect, it, vi } from "vitest";
import {
  GetCurrentUserResponseSchema,
  UserAppService,
} from "@/generated/app/users/v1/users_pb";
import { connectResponse, requestOf } from "./connect-wire";

vi.mock("server-only", () => ({}));

// A server-side call forwards the visitor's session cookie. Without the site's
// URL it must not fall back to a real host, or the cookie would go there.
vi.stubGlobal("window", undefined);
vi.stubEnv("NODE_ENV", "production");
vi.stubEnv("NEXT_PUBLIC_APP_URL", "");

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const { serverTransport } = await import("../server-transport");

it("sends server-side calls to an unresolvable host when the site URL is unset", async () => {
  mockFetch.mockResolvedValueOnce(
    connectResponse(GetCurrentUserResponseSchema, {}),
  );

  await createClient(UserAppService, serverTransport).getCurrentUser(
    {},
    { headers: { cookie: "session=abc" } },
  );

  expect(requestOf(mockFetch.mock.calls[0]).url.hostname).toMatch(/\.invalid$/);
});
