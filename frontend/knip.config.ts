import type { KnipConfig } from "knip";

const config: KnipConfig = {
  ignore: ["src/test-setup.ts"],
  ignoreDependencies: [],
  ignoreBinaries: [],
  compilers: {
    css: (text: string) => [...text.matchAll(/(?<=@)import[^;]+/g)].join("\n"),
  },
};

export default config;
