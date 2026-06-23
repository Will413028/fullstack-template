import { type NextRequest, NextResponse } from "next/server";
import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

const intlMiddleware = createMiddleware(routing);

// Anchored to a full segment so /enroll, /registry etc. aren't mistaken for the
// "en" locale prefix.
const localePrefix = new RegExp(`^/(${routing.locales.join("|")})(?=/|$)`);

const protectedPaths = ["/overview"];
const authPaths = ["/login"];

// Browser-facing URL for the CSP connect-src.
const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";
// Server-side URL (the backend service name inside Docker) for the refresh call.
const serverApiUrl =
  process.env.API_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8000";

const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  `connect-src 'self' ${apiUrl}`,
  "frame-ancestors 'none'",
].join("; ");

// Proxy (Next.js 16 Node-runtime convention, formerly "middleware"): keeps the
// session fresh and bounces unauthenticated requests for snappy UX + CSP headers.
// It is NOT the security boundary — the real session check runs server-side in
// the (dashboard) layout via getServerUser().
export default async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const pathnameWithoutLocale = pathname.replace(localePrefix, "") || "/";
  const locale = pathname.match(localePrefix)?.[1] || routing.defaultLocale;

  const access = request.cookies.get("access_token")?.value;
  const refresh = request.cookies.get("refresh_token")?.value;
  const isProtected = protectedPaths.some((p) =>
    pathnameWithoutLocale.startsWith(p),
  );
  const isAuthPage = authPaths.some((p) => pathnameWithoutLocale.startsWith(p));

  if (isProtected && !access) {
    // Access token expired/absent. If a refresh token is present, transparently
    // rotate via the backend and reload the same URL with the new cookies, so a
    // session lives as long as the refresh token (7 days) rather than the short
    // access token (30 min) on hard navigations / SSR.
    if (refresh) {
      const refreshed = await fetch(new URL("/auth/refresh", serverApiUrl), {
        method: "POST",
        headers: { cookie: request.headers.get("cookie") ?? "" },
      }).catch(() => null);

      if (refreshed?.ok) {
        const res = NextResponse.redirect(request.url);
        for (const cookie of refreshed.headers.getSetCookie()) {
          res.headers.append("set-cookie", cookie);
        }
        return res;
      }
    }
    const loginUrl = new URL(`/${locale}/login`, request.url);
    loginUrl.searchParams.set("callbackUrl", pathnameWithoutLocale);
    return NextResponse.redirect(loginUrl);
  }

  if (isAuthPage && access) {
    return NextResponse.redirect(new URL(`/${locale}/overview`, request.url));
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
