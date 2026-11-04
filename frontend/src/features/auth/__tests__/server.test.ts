import { create } from "@bufbuild/protobuf";
import { Code } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorReason } from "@/generated/app/common/v1/errors_pb";
import {
  GetCurrentUserResponseSchema,
  Role,
  UserSchema,
} from "@/generated/app/users/v1/users_pb";
import {
  connectError,
  connectResponse,
  requestOf,
} from "@/lib/api/__tests__/connect-wire";

vi.mock("server-only", () => ({}));

const mockCookies = vi.fn();
vi.mock("next/headers", () => ({ cookies: () => mockCookies() }));

// Server-side code has no page origin; it calls /api on the site's own URL, so
// the request goes through Vercel's edge like the browser's.
vi.stubGlobal("window", undefined);
vi.stubEnv("NEXT_PUBLIC_APP_URL", "https://app.example.com");

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const { getServerUser } = await import("../server");

const user = create(UserSchema, {
  id: "0199c0de-0000-7000-8000-000000000001",
  account: "alice",
  role: Role.USER,
  disabled: false,
});

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

  it("forwards only the request cookies with a GET to GetCurrentUser and returns the user", async () => {
    withCookies("session=abc");
    mockFetch.mockResolvedValueOnce(
      connectResponse(GetCurrentUserResponseSchema, { user }),
    );

    expect(await getServerUser()).toEqual(user);
    const request = requestOf(mockFetch.mock.calls[0]);
    // A read marked NO_SIDE_EFFECTS goes out as a GET, never cached: the
    // answer depends on the cookie.
    expect(request.method).toBe("GET");
    expect(request.url.origin + request.url.pathname).toBe(
      "https://app.example.com/api/app.users.v1.UserAppService/GetCurrentUser",
    );
    expect(request.cache).toBe("no-store");
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
      connectError(Code.Unauthenticated, "session is invalid", {
        reason: ErrorReason.SESSION_INVALID,
      }),
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
    mockFetch.mockResolvedValueOnce(connectError(Code.Internal, "boom"));

    await expect(getServerUser()).rejects.toMatchObject({
      code: Code.Internal,
    });
  });
});
