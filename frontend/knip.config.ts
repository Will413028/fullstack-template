import type { KnipConfig } from "knip";

const config: KnipConfig = {
  // Template public API that ships before any page consumes it: feature
  // barrels, shadcn's cn() helper, and shared hooks.
  entry: ["src/features/*/index.ts", "src/lib/utils.ts", "src/hooks/*.ts"],
  ignoreDependencies: [],
  ignoreBinaries: [],
  compilers: {
    css: (text: string) =>
      [...text.matchAll(/@(?:import|plugin)\s+([^;\s]+)/g)]
        .map(([, specifier]) => `import ${specifier};`)
        .join("\n"),
  },
};

export default config;
