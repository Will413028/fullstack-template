import { beforeEach, describe, expect, it, vi } from "vitest";

// Must set env before importing api-client
vi.stubEnv("NEXT_PUBLIC_API_URL", "http://localhost:8000");

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
      json: () => Promise.resolve({ data: {} }),
    });

    const params = new URLSearchParams();
    params.append("username", "test");
    params.append("password", "pass");
    await apiClient.post("/auth/login", params);

    const [, options] = mockFetch.mock.calls[0];
    expect(options.body).toBeInstanceOf(URLSearchParams);
    expect(options.headers.has("Content-Type")).toBe(false);
  });

  it("always sends credentials so httpOnly auth cookies are included", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await apiClient.get("/test");
    const [, options] = mockFetch.mock.calls[0];
    expect(options.credentials).toBe("include");
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
});

describe("401 auto-refresh", () => {
  it("refreshes session and retries on 401 for non-auth endpoints", async () => {
    // First call: 401
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Token expired" }),
    });

    // Refresh call: success (backend rotates httpOnly cookies)
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: { id: 1 } }),
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

    // The refresh call hits /auth/refresh with credentials and no body
    const [refreshUrl, refreshOpts] = mockFetch.mock.calls[1];
    expect(refreshUrl).toContain("/auth/refresh");
    expect(refreshOpts.method).toBe("POST");
    expect(refreshOpts.credentials).toBe("include");
  });

  it("redirects to login when refresh fails", async () => {
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

    const originalLocation = window.location;
    Object.defineProperty(window, "location", {
      writable: true,
      value: { href: "" },
    });

    await expect(apiClient.get("/items")).rejects.toThrow(ApiError);
    expect(window.location.href).toBe("/login");

    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
  });

  it("does not attempt refresh for /auth/ endpoints", async () => {
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
});
