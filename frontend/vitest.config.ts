import react from "@vitejs/plugin-react";
import { resolve } from "path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    // next-intl's middleware imports "next/server" without an extension, which
    // Node's ESM resolver rejects; inlining lets Vite resolve it (proxy.test.ts).
    server: { deps: { inline: ["next-intl"] } },
  },
});
