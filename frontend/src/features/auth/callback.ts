const fallback = "/overview";
const base = "http://app.invalid";

// Where to go after signing in: callbackUrl when it is a path on this site, else the overview. The URL parser reads
// the value the way the browser will, so one that only looks local ("/\evil.example", "/\t/evil.example") resolves
// to another origin and is refused.
export function safeCallbackPath(callbackUrl: string | null): string {
  if (!callbackUrl?.startsWith("/")) return fallback;
  let url: URL;
  try {
    url = new URL(callbackUrl, base);
  } catch {
    return fallback;
  }
  return url.origin === base ? url.pathname + url.search + url.hash : fallback;
}
