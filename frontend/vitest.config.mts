import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    // Random file and test order, so a test that depends on another's leftovers fails; the run prints its seed, and
    // --sequence.seed=<seed> replays that order.
    sequence: { shuffle: true },
    // next-intl's middleware imports "next/server" without an extension, which
    // Node's ESM resolver rejects; inlining lets Vite resolve it (proxy.test.ts).
    server: { deps: { inline: ["next-intl"] } },
  },
});
