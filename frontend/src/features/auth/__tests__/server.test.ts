import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

const mockCookies = vi.fn();
vi.mock("next/headers", () => ({ cookies: () => mockCookies() }));

// Server-side code has no page origin; the client must take the API_URL branch.
vi.stubGlobal("window", undefined);
vi.stubEnv("API_URL", "http://backend:8000");

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
    const request = mockFetch.mock.calls[0][0];
    expect(request.url).toBe("http://backend:8000/api/users/me");
    expect(request.headers.get("cookie")).toBe("session=abc");
    // Next is a trusted proxy: forwarding X-Forwarded-For would let the client
    // choose the backend's rate-limit key.
    expect(request.headers.has("x-forwarded-for")).toBe(false);
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
