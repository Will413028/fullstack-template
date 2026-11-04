// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// These packages call React's createContext when they load and carry no "use client", so Next.js evaluates them with
// the server build of React, which has no createContext, whenever a server component reaches them: a page importing a
// feature's index.ts, which re-exports that feature's hooks, then fails with "createContext is not a function" under
// next dev. A module that loads one must itself be a client module. Type-only imports are erased and load nothing.
const contextAtLoad = ["@connectrpc/connect-query", "react-hook-form"];

const src = fileURLToPath(new URL("..", import.meta.url));

function sources(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "__tests__" ? [] : sources(path);
    }
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)
      ? [path]
      : [];
  });
}

const files = sources(src).map((path) => ({
  name: relative(src, path).split("\\").join("/"),
  code: readFileSync(path, "utf8"),
}));

// The directive must come before any statement; comments may precede it.
const useClient =
  /^(?:\s*\/\/[^\n]*\n|\s*\/\*[\s\S]*?\*\/)*\s*["']use client["']/;

// A value import or re-export, a side-effect import, or a dynamic import of pkg. `import type { … }` and an import
// whose specifiers are all `type …` are erased.
function loads(code: string, pkg: string): boolean {
  const name = `["']${pkg.replace(/[/.]/g, "\\$&")}["']`;
  const statements = new RegExp(
    `^(?:import|export)\\s+(?!type\\b)([^;]*?)\\s*from\\s+${name}`,
    "gm",
  );
  for (const [, clause] of code.matchAll(statements)) {
    const specifiers = /^\{([\s\S]*)\}$/.exec((clause ?? "").trim())?.[1];
    const allTypes = specifiers
      ?.split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .every((s) => s.startsWith("type "));
    if (!allTypes) return true;
  }
  return [`^import\\s+${name}`, `\\bimport\\(\\s*${name}\\s*\\)`].some(
    (pattern) => new RegExp(pattern, "m").test(code),
  );
}

describe("client boundary", () => {
  it("every module that loads a package creating a React context at load is a client module", () => {
    const serverReachable = files
      .filter(
        ({ code }) =>
          contextAtLoad.some((pkg) => loads(code, pkg)) &&
          !useClient.test(code),
      )
      .map(({ name }) => name);
    expect(serverReachable).toEqual([]);
  });

  it("finds the modules that load each package", () => {
    // if the scan found nothing, the check above would pass vacuously
    const loaders = (pkg: string) =>
      files.filter(({ code }) => loads(code, pkg)).map(({ name }) => name);
    expect(loaders("@connectrpc/connect-query")).toContain(
      "providers/query-provider.tsx",
    );
    expect(loaders("react-hook-form")).toContain(
      "features/auth/components/auth-form.tsx",
    );
    expect(
      loads(
        'import { type A, type B } from "react-hook-form";',
        "react-hook-form",
      ),
    ).toBe(false);
  });
});
