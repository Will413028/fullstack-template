import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

const mockCookies = vi.fn();
vi.mock("next/headers", () => ({ cookies: () => mockCookies() }));

vi.stubEnv("API_URL", "http://backend:8000");

const mockFetch = vi.fn();
global.fetch = mockFetch;

const { getServerUser } = await import("../server");

const user = { id: 1, email: "a@example.com" };

function withCookies(header: string) {
  mockCookies.mockResolvedValue({ toString: () => header });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("getServerUser", () => {
  it("returns null without calling the backend when there are no cookies", async () => {
    withCookies("");

    expect(await getServerUser()).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("forwards request cookies to /auth/me and returns the user", async () => {
    withCookies("access_token=abc");
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ data: user }),
    });

    expect(await getServerUser()).toEqual(user);
    expect(mockFetch).toHaveBeenCalledWith("http://backend:8000/auth/me", {
      headers: { cookie: "access_token=abc" },
      cache: "no-store",
    });
  });

  it("returns null when the backend rejects the session", async () => {
    withCookies("access_token=expired");
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () => Promise.resolve({ data: user }),
    });

    expect(await getServerUser()).toBeNull();
  });

  it("returns null when the backend is unreachable", async () => {
    withCookies("access_token=abc");
    mockFetch.mockRejectedValueOnce(new TypeError("fetch failed"));

    expect(await getServerUser()).toBeNull();
  });
});
