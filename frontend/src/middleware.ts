import { type NextRequest, NextResponse } from "next/server";
import createMiddleware from "next-intl/middleware";
import { routing } from "@/i18n/routing";

const intlMiddleware = createMiddleware(routing);

const localePrefix = new RegExp(`^/(${routing.locales.join("|")})`);

const protectedPaths = ["/overview"];
const authPaths = ["/login", "/register", "/forgot-password"];

const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";

const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https://*.unsplash.com",
  "font-src 'self'",
  `connect-src 'self' ${apiUrl}`,
  "frame-ancestors 'none'",
].join("; ");

export default function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;

  const pathnameWithoutLocale = pathname.replace(localePrefix, "") || "/";

  const token = request.cookies.get("auth_token")?.value;
  const isProtected = protectedPaths.some((p) =>
    pathnameWithoutLocale.startsWith(p),
  );
  const isAuthPage = authPaths.some((p) => pathnameWithoutLocale.startsWith(p));

  if (isProtected && !token) {
    const locale = pathname.match(localePrefix)?.[1] || routing.defaultLocale;
    const loginUrl = new URL(`/${locale}/login`, request.url);
    loginUrl.searchParams.set("callbackUrl", pathnameWithoutLocale);
    return NextResponse.redirect(loginUrl);
  }

  if (isAuthPage && token) {
    const locale = pathname.match(localePrefix)?.[1] || routing.defaultLocale;
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
