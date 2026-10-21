import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// openapi-fetch captures fetch when the client is created, so stub it first.
const mockFetch = vi.fn<(request: Request) => Promise<Response>>();
vi.stubGlobal("fetch", mockFetch);

const { api } = await import("../client");
const { ApiError, unwrap } = await import("../errors");

function problem(status: number, body: Record<string, unknown>) {
  return new Response(JSON.stringify({ status, ...body }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

const originalLocation = window.location;

beforeEach(() => {
  mockFetch.mockReset();
  Object.defineProperty(window, "location", {
    writable: true,
    value: { href: "", origin: originalLocation.origin },
  });
});

afterEach(() => {
  Object.defineProperty(window, "location", {
    writable: true,
    value: originalLocation,
  });
});

describe("api client", () => {
  it("calls the API on the page's own origin under /api", async () => {
    mockFetch.mockResolvedValueOnce(Response.json({ id: "u1" }));

    await api.GET("/users/me");

    const [request] =
      mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
    expect(request.url).toBe(`${originalLocation.origin}/api/users/me`);
  });

  it("sends a JSON body", async () => {
    mockFetch.mockResolvedValueOnce(
      Response.json({ user: {} }, { status: 201 }),
    );

    await api.POST("/sessions", { body: { account: "a", password: "p" } });

    const [request] =
      mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
    expect(request.method).toBe("POST");
    expect(request.headers.get("Content-Type")).toBe("application/json");
    expect(await request.text()).toBe('{"account":"a","password":"p"}');
  });

  it("sends the user to login on a 401 without trying to refresh", async () => {
    mockFetch.mockResolvedValueOnce(
      problem(401, {
        type: "/problems/unauthenticated",
        title: "Unauthenticated",
      }),
    );

    await expect(unwrap(api.GET("/users/me"))).rejects.toBeInstanceOf(ApiError);

    expect(window.location.href).toBe("/login");
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("leaves a 401 from signing in to the form", async () => {
    mockFetch.mockResolvedValueOnce(
      problem(401, {
        type: "/problems/unauthenticated",
        title: "Unauthenticated",
        code: "invalid_credentials",
      }),
    );

    await expect(
      unwrap(api.POST("/sessions", { body: { account: "a", password: "p" } })),
    ).rejects.toMatchObject({ problem: { code: "invalid_credentials" } });

    expect(window.location.href).toBe("");
  });
});

describe("unwrap", () => {
  it("returns the data, and undefined for 204", async () => {
    mockFetch.mockResolvedValueOnce(Response.json({ id: "u1" }));
    await expect(unwrap(api.GET("/users/me"))).resolves.toEqual({ id: "u1" });

    mockFetch.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await expect(
      unwrap(api.DELETE("/sessions/current")),
    ).resolves.toBeUndefined();
  });

  it("turns a body that is not a problem into a generic one", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response("<html>Bad Gateway</html>", {
        status: 502,
        statusText: "Bad Gateway",
      }),
    );

    await expect(unwrap(api.GET("/users/me"))).rejects.toMatchObject({
      status: 502,
      problem: { type: "about:blank", status: 502 },
    });
  });
});
