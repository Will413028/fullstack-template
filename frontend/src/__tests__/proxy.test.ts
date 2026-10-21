// @vitest-environment node
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import proxy from "../proxy";

function get(path: string, cookie?: string) {
  return proxy(
    new NextRequest(`http://localhost${path}`, {
      headers: cookie ? { cookie } : {},
    }),
  );
}

function isLoginRedirect(response: Response) {
  const location = response.headers.get("location");
  return (
    response.status === 307 &&
    location !== null &&
    new URL(location).pathname === "/en/login"
  );
}

describe("proxy", () => {
  it("sends a protected page without a session cookie to login", () => {
    const response = get("/en/overview");
    expect(isLoginRedirect(response)).toBe(true);
    expect(
      new URL(response.headers.get("location") ?? "").searchParams.get(
        "callbackUrl",
      ),
    ).toBe("/overview");
  });

  it.each([["__Host-session=abc"], ["session=abc"]])(
    "lets a protected page through with %s",
    (cookie) => {
      expect(isLoginRedirect(get("/en/overview", cookie))).toBe(false);
    },
  );

  it("does not take another cookie for a session", () => {
    expect(
      isLoginRedirect(get("/en/overview", "NEXT_LOCALE=en; theme=dark")),
    ).toBe(true);
  });

  it("does not send the login page on with a cookie that may be stale", () => {
    const response = get("/en/login", "session=stale");
    expect(response.headers.get("location") ?? "").not.toContain("/overview");
  });

  it("allows connecting only to its own origin", () => {
    const csp = get("/en").headers.get("content-security-policy") ?? "";
    expect(csp).toContain("connect-src 'self';");
  });

  it("allows only scripts with a fresh nonce, which it passes on to rendering", () => {
    const nonceOf = (response: Response) =>
      /'nonce-([^']+)'/.exec(
        response.headers.get("content-security-policy") ?? "",
      )?.[1];
    const response = get("/en");
    const nonce = nonceOf(response);
    expect(nonce).toBeTruthy();
    expect(nonceOf(get("/en"))).not.toBe(nonce);
    // Next reads the request's CSP header to put the nonce on its scripts
    expect(
      response.headers.get("x-middleware-request-content-security-policy"),
    ).toContain(`'nonce-${nonce}'`);
  });
});
