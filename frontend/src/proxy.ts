import { NextRequest, NextResponse } from "next/server";
import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

const intlMiddleware = createMiddleware(routing);

// Anchored to a full segment so /enroll, /registry etc. aren't mistaken for the
// "en" locale prefix.
const localePrefix = new RegExp(`^/(${routing.locales.join("|")})(?=/|$)`);

const protectedPaths = ["/overview"];

// The backend names the cookie __Host-session, or session when it runs with
// COOKIE_SECURE=false for local http (the __Host- prefix requires Secure).
const sessionCookies = ["__Host-session", "session"];

// Next.js renders inline scripts to boot the page, so script-src needs this
// request's nonce: Next reads it from the request's CSP header and puts it on
// its scripts (Next.js guide "Content Security Policy"). Without it no page
// ever hydrates and forms fall back to a plain submit.
function cspFor(nonce: string): string {
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self'",
    // the API is same-origin under /api
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
  ].join("; ");
}

// Proxy (Next.js 16 Node-runtime convention, formerly "middleware"): bounces
// requests for protected pages that carry no session cookie, for snappy UX,
// and sets the CSP. It is NOT the security boundary — the cookie may be stale,
// and the real session check runs server-side in the (dashboard) layout via
// getServerUser(). For the same reason a cookie does not send the login page
// on to /overview: with a stale cookie the two would redirect to each other.
export default function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const pathnameWithoutLocale = pathname.replace(localePrefix, "") || "/";
  const locale = pathname.match(localePrefix)?.[1] || routing.defaultLocale;

  const hasSession = sessionCookies.some((name) => request.cookies.has(name));
  const isProtected = protectedPaths.some((p) =>
    pathnameWithoutLocale.startsWith(p),
  );

  if (isProtected && !hasSession) {
    const loginUrl = new URL(`/${locale}/login`, request.url);
    loginUrl.searchParams.set("callbackUrl", pathnameWithoutLocale);
    return NextResponse.redirect(loginUrl);
  }

  // next dev needs eval and its own inline scripts; the policy is for builds.
  if (process.env.NODE_ENV === "development") return intlMiddleware(request);

  const csp = cspFor(btoa(crypto.randomUUID()));
  const headers = new Headers(request.headers);
  headers.set("Content-Security-Policy", csp);
  const response = intlMiddleware(new NextRequest(request, { headers }));
  response.headers.set("Content-Security-Policy", csp);
  return response;
}

export const config = {
  matcher: ["/((?!api|_next|_vercel|.*\\..*).*)"],
};
