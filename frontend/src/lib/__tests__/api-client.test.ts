import { beforeEach, describe, expect, it, vi } from "vitest";

// Must set env before importing api-client
vi.stubEnv("NEXT_PUBLIC_API_URL", "http://localhost:8000");

vi.mock("../cookies", () => ({
  getCookie: vi.fn(() => null),
  setCookie: vi.fn(),
  removeCookie: vi.fn(),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

const { apiClient, ApiError } = await import("../api-client");

beforeEach(() => {
  vi.clearAllMocks();
});

describe("apiClient", () => {
  it("makes GET request with correct URL", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: "test" }),
    });

    const result = await apiClient.get("/test");
    expect(result).toEqual({ data: "test" });
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining("/test"),
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("makes POST request with JSON body", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: 1 }),
    });

    await apiClient.post("/items", { title: "Test" });
    const [, options] = mockFetch.mock.calls[0];
    expect(options.method).toBe("POST");
    expect(options.body).toBe('{"title":"Test"}');
    expect(options.headers.get("Content-Type")).toBe("application/json");
  });

  it("sends URLSearchParams body without JSON content-type", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ token: "abc" }),
    });

    const params = new URLSearchParams();
    params.append("username", "test");
    params.append("password", "pass");
    await apiClient.post("/auth/login", params);

    const [, options] = mockFetch.mock.calls[0];
    expect(options.body).toBeInstanceOf(URLSearchParams);
    expect(options.headers.has("Content-Type")).toBe(false);
  });

  it("throws ApiError on non-OK response for auth endpoints (no refresh attempt)", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Invalid credentials" }),
    });

    await expect(apiClient.post("/auth/login", {})).rejects.toThrow(ApiError);
    // Only one fetch call — no refresh attempt for /auth/ paths
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("parses error detail from backend response", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      statusText: "Not Found",
      json: () => Promise.resolve({ detail: "Item not found" }),
    });

    try {
      await apiClient.get("/items/999");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      const err = e as InstanceType<typeof ApiError>;
      expect(err.status).toBe(404);
      expect(err.message).toBe("Item not found");
    }
  });

  it("handles 204 No Content", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 204,
    });

    const result = await apiClient.delete("/items/1");
    expect(result).toBeUndefined();
  });

  it("includes auth token from cookie when available", async () => {
    const { getCookie } = await import("../cookies");
    vi.mocked(getCookie).mockReturnValue("test-token-123");

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await apiClient.get("/auth/me");
    const [, options] = mockFetch.mock.calls[0];
    expect(options.headers.get("Authorization")).toBe("Bearer test-token-123");
  });

  it("includes credentials for cookie support", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await apiClient.get("/test");
    const [, options] = mockFetch.mock.calls[0];
    expect(options.credentials).toBe("include");
  });
});

describe("401 auto-refresh", () => {
  it("refreshes token and retries on 401 for non-auth endpoints", async () => {
    const { getCookie, setCookie } = await import("../cookies");
    vi.mocked(getCookie)
      .mockReturnValueOnce("expired-token") // first request: auth_token
      .mockReturnValueOnce("valid-refresh-token") // refresh: refresh_token
      .mockReturnValueOnce("new-access-token"); // retry: auth_token

    // First call: 401
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Token expired" }),
    });

    // Refresh call: success
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          access_token: "new-access-token",
          refresh_token: "new-refresh-token",
        }),
    });

    // Retry call: success
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: "success" }),
    });

    const result = await apiClient.get("/items");
    expect(result).toEqual({ data: "success" });
    expect(mockFetch).toHaveBeenCalledTimes(3);
    expect(setCookie).toHaveBeenCalledWith("auth_token", "new-access-token", 1);
    expect(setCookie).toHaveBeenCalledWith(
      "refresh_token",
      "new-refresh-token",
      7,
    );
  });

  it("redirects to login when refresh fails", async () => {
    const { getCookie, removeCookie } = await import("../cookies");
    vi.mocked(getCookie)
      .mockReturnValueOnce("expired-token") // first request: auth_token
      .mockReturnValueOnce("invalid-refresh-token"); // refresh: refresh_token

    // Original request: 401
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Token expired" }),
    });

    // Refresh call: fails
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Invalid refresh token" }),
    });

    // Mock window.location
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { href: "" },
    });

    await expect(apiClient.get("/items")).rejects.toThrow(ApiError);
    expect(removeCookie).toHaveBeenCalledWith("auth_token");
    expect(removeCookie).toHaveBeenCalledWith("refresh_token");
    expect(window.location.href).toBe("/login");

    // Restore
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
  });

  it("does not attempt refresh for /auth/ endpoints", async () => {
    const { getCookie } = await import("../cookies");
    vi.mocked(getCookie).mockReturnValue("expired-token");

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Invalid credentials" }),
    });

    await expect(apiClient.get("/auth/me")).rejects.toThrow(ApiError);
    // Only one fetch — no refresh attempt
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("does not refresh when no refresh token exists", async () => {
    const { getCookie } = await import("../cookies");
    vi.mocked(getCookie)
      .mockReturnValueOnce("expired-token") // first request: auth_token
      .mockReturnValueOnce(null); // refresh: no refresh_token

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Token expired" }),
    });

    // Mock window.location
    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { href: "" },
    });

    await expect(apiClient.get("/items")).rejects.toThrow(ApiError);
    // Only 1 fetch — no refresh attempt (no refresh token)
    expect(mockFetch).toHaveBeenCalledTimes(1);

    // Restore
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
  });
});
