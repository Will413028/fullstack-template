// Layering for src/ (see "Dependency direction" in AGENTS.md). Every rule is an error and there are no exceptions:
// fix a violation by moving code, not by adding a known-violations file.
/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "no-circular",
      comment:
        "An import cycle ties its modules into one unit that cannot change or be tested apart.",
      severity: "error",
      from: {},
      to: { circular: true },
    },
    {
      name: "shared-not-to-features-or-app",
      comment:
        "lib, components, hooks, store, providers and i18n are shared by every feature and never depend on one.",
      severity: "error",
      from: { path: "^src/(lib|components|hooks|store|providers|i18n)/" },
      to: { path: "^src/(features|app)/" },
    },
    {
      name: "features-not-to-app",
      comment: "Routes compose features; a feature never imports a route.",
      severity: "error",
      from: { path: "^src/features/" },
      to: { path: "^src/app/" },
    },
    {
      name: "feature-public-api-from-outside",
      comment:
        "Outside a feature, import it through index.ts, or server.ts for server-only code.",
      severity: "error",
      from: { pathNot: "^src/features/" },
      to: {
        path: "^src/features/[^/]+/",
        pathNot: "^src/features/[^/]+/(index|server)\\.ts$",
      },
    },
    {
      name: "feature-public-api-between-features",
      comment:
        "One feature reaches another only through its index.ts or server.ts.",
      severity: "error",
      from: { path: "^src/features/([^/]+)/" },
      to: {
        path: "^src/features/[^/]+/",
        pathNot: [
          "^src/features/$1/",
          "^src/features/[^/]+/(index|server)\\.ts$",
        ],
      },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    tsConfig: { fileName: "tsconfig.json" },
    tsPreCompilationDeps: true,
    enhancedResolveOptions: {
      exportsFields: ["exports"],
      conditionNames: ["import", "require", "node", "default", "types"],
    },
  },
};
