// The one module that reads process.env; Biome's noProcessEnv is off for this file only. Each value is a literal
// `process.env.X` read inside a function: Next.js inlines NEXT_PUBLIC_* and NODE_ENV only for literal reads, and
// server-only values such as API_URL are read at call time, not when a client bundle loads this module.

export const isDevelopment = () => process.env.NODE_ENV === "development";

export const appUrl = () =>
  process.env.NEXT_PUBLIC_APP_URL || "https://example.com";

export const apiUrl = () => process.env.API_URL || "http://localhost:8000";
