import type { KnipConfig } from "knip";

const config: KnipConfig = {
  // Template public API that ships before any page consumes it: feature
  // barrels, shadcn's cn() helper, and shared hooks.
  entry: ["src/features/*/index.ts", "src/lib/utils.ts", "src/hooks/*.ts"],
  // Generated from the contract (buf.gen.yaml): each file exports every message and enum of its proto, and the app
  // uses them piecemeal. Whether a generated file is current is pnpm gen:api:check's job, not knip's.
  ignore: ["src/generated/**"],
  ignoreDependencies: [],
  // gen:api builds buf through the root Makefile (make bin/buf), the one place its version and checksums live.
  ignoreBinaries: ["make"],
  compilers: {
    css: (text: string) =>
      [...text.matchAll(/@(?:import|plugin)\s+([^;\s]+)/g)]
        .map(([, specifier]) => `import ${specifier};`)
        .join("\n"),
  },
};

export default config;
