import { expect, it, vi } from "vitest";

// A server-side call forwards the visitor's session cookie. Without the site's URL it must not fall back to a real
// host, or the cookie would go there (server-base.test.ts checks the same through a call on the server transport).
vi.stubGlobal("window", undefined);
vi.stubEnv("NODE_ENV", "production");
vi.stubEnv("NEXT_PUBLIC_APP_URL", "");

const { apiBaseUrl } = await import("../base-url");

it("sends server-side calls to an unresolvable host when the site URL is unset", () => {
  expect(new URL(apiBaseUrl()).hostname).toMatch(/\.invalid$/);
});
