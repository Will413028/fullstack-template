import { notFound } from "next/navigation";

// An unknown path under a locale renders [locale]/not-found.tsx inside the locale layout, rendered per request, so
// its scripts carry the request's CSP nonce. Without this route Next.js answers with its built-in 404, prerendered at
// build time: not one of its scripts has the nonce, so it never hydrates (scripts/check_built_pages.py).
export default function UnknownPage() {
  notFound();
}
