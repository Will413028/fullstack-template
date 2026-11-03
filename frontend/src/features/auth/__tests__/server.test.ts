import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

const mockCookies = vi.fn();
vi.mock("next/headers", () => ({ cookies: () => mockCookies() }));

// Server-side code has no page origin; it calls /api on the site's own URL, so
// the request goes through Vercel's edge like the browser's.
vi.stubGlobal("window", undefined);
vi.stubEnv("NEXT_PUBLIC_APP_URL", "https://app.example.com");

const mockFetch = vi.fn<(request: Request) => Promise<Response>>();
vi.stubGlobal("fetch", mockFetch);

const { getServerUser } = await import("../server");

const user = { id: "0199c0de-0000-7000-8000-000000000001", account: "alice" };

function withCookies(header: string) {
  mockCookies.mockResolvedValue({ toString: () => header });
}

beforeEach(() => {
  mockFetch.mockReset();
});

describe("getServerUser", () => {
  it("returns null without calling the backend when there are no cookies", async () => {
    withCookies("");

    expect(await getServerUser()).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("forwards only the request cookies to /api/users/me and returns the user", async () => {
    withCookies("session=abc");
    mockFetch.mockResolvedValueOnce(Response.json(user));

    expect(await getServerUser()).toEqual(user);
    const [request] =
      mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
    expect(request.url).toBe("https://app.example.com/api/users/me");
    expect(request.headers.get("cookie")).toBe("session=abc");
    // Next holds no origin secret and sets no client address: Vercel's edge
    // writes X-Real-IP and the origin secret (docs/adr/2026-11-03-frontend-on-vercel.md).
    expect(request.headers.has("x-forwarded-for")).toBe(false);
    expect(request.headers.has("x-real-ip")).toBe(false);
    expect(request.headers.has("x-origin-secret")).toBe(false);
  });

  it("returns null when the backend rejects the session", async () => {
    withCookies("session=expired");
    mockFetch.mockResolvedValueOnce(
      Response.json({ type: "/problems/unauthenticated" }, { status: 401 }),
    );

    expect(await getServerUser()).toBeNull();
  });

  it("throws instead of signing the user out when the backend is unreachable", async () => {
    withCookies("session=abc");
    mockFetch.mockRejectedValueOnce(new TypeError("fetch failed"));

    await expect(getServerUser()).rejects.toThrow("fetch failed");
  });

  it("throws on a server error", async () => {
    withCookies("session=abc");
    mockFetch.mockResolvedValueOnce(
      Response.json(
        { type: "/problems/internal", title: "Internal error", status: 500 },
        { status: 500 },
      ),
    );

    await expect(getServerUser()).rejects.toMatchObject({ status: 500 });
  });
});
