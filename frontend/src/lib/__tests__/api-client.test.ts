import { beforeEach, describe, expect, it, vi } from "vitest";

// Must set env before importing api-client
vi.stubEnv("NEXT_PUBLIC_API_URL", "http://localhost:8000");

vi.mock("../cookies", () => ({
  getCookie: vi.fn(() => null),
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

  it("throws ApiError on non-OK response", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      json: () => Promise.resolve({ detail: "Invalid credentials" }),
    });

    await expect(apiClient.get("/auth/me")).rejects.toThrow(ApiError);
    await expect(apiClient.get("/auth/me")).rejects.toThrow(); // second call to ensure pattern
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
