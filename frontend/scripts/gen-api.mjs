// Generates src/lib/api/schema.d.ts from the contract with openapi-typescript, which leaves x-extensible-enum lists
// out: their values become string literal unions here, so messages.test.ts can check that every listed problem
// type and code has a message. The union names the values this client knows; errors.ts falls back for others.
//
//   node scripts/gen-api.mjs          write the file
//   node scripts/gen-api.mjs --check  fail when the file differs
import { readFile, writeFile } from "node:fs/promises";
import openapiTS, {
  astToString,
  COMMENT_HEADER,
  tsLiteral,
  tsUnion,
} from "openapi-typescript";

const contract = new URL("../../backend/api/openapi.yaml", import.meta.url);
const target = new URL("../src/lib/api/schema.d.ts", import.meta.url);

const ast = await openapiTS(contract, {
  transform(schema) {
    const values = schema["x-extensible-enum"];
    if (!Array.isArray(values)) return undefined;
    return tsUnion(values.map((v) => tsLiteral(v)));
  },
});
const output = COMMENT_HEADER + astToString(ast);

if (process.argv.includes("--check")) {
  const current = await readFile(target, "utf8").catch(() => "");
  if (current !== output) {
    console.error("src/lib/api/schema.d.ts is stale: run pnpm gen:api");
    process.exit(1);
  }
} else {
  await writeFile(target, output);
}
