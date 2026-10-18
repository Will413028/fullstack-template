import { type NextRequest, NextResponse } from "next/server";
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

const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  // the API is same-origin under /api
  "connect-src 'self'",
  "frame-ancestors 'none'",
].join("; ");

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

  const response = intlMiddleware(request);

  if (process.env.NODE_ENV !== "development") {
    response.headers.set("Content-Security-Policy", csp);
  }

  return response;
}

export const config = {
  matcher: ["/((?!api|_next|_vercel|.*\\..*).*)"],
};
