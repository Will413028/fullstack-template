import { describe, expect, it } from "vitest";
import { safeCallbackPath } from "../callback";

describe("safeCallbackPath", () => {
  it.each([
    ["/settings", "/settings"],
    ["/en/items?page=2#top", "/en/items?page=2#top"],
  ])("keeps the local path %s", (input, want) => {
    expect(safeCallbackPath(input)).toBe(want);
  });

  // Each of these leaves the site once the browser parses it.
  it.each([
    null,
    "",
    "settings",
    "//evil.example",
    "/\\evil.example",
    "/\t/evil.example",
    "https://evil.example/",
    "javascript:alert(1)",
  ])("sends %j to the overview", (input) => {
    expect(safeCallbackPath(input)).toBe("/overview");
  });
});
