import { expect, it, vi } from "vitest";

// A server-side call forwards the visitor's session cookie. Without the site's
// URL it must not fall back to a real host (the sitemap's example.com), or the
// cookie would go there.
vi.stubGlobal("window", undefined);
vi.stubEnv("NODE_ENV", "production");
vi.stubEnv("NEXT_PUBLIC_APP_URL", "");

const mockFetch = vi.fn<(request: Request) => Promise<Response>>();
vi.stubGlobal("fetch", mockFetch);

const { api } = await import("../client");

it("sends server-side calls to an unresolvable host when the site URL is unset", async () => {
  mockFetch.mockResolvedValueOnce(Response.json({}));

  await api.GET("/users/me", { headers: { cookie: "session=abc" } });

  const [request] =
    mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
  expect(new URL(request.url).hostname).toMatch(/\.invalid$/);
});
