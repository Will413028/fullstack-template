import { beforeEach, describe, expect, it } from "vitest";
import { getCookie, removeCookie, setCookie } from "../cookies";

beforeEach(() => {
  // Clear all cookies
  document.cookie.split(";").forEach((c) => {
    const name = c.split("=")[0].trim();
    if (name) {
      document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
    }
  });
});

describe("cookies", () => {
  it("setCookie and getCookie round-trip", () => {
    setCookie("test_key", "test_value", 1);
    expect(getCookie("test_key")).toBe("test_value");
  });

  it("getCookie returns null for missing cookie", () => {
    expect(getCookie("nonexistent")).toBeNull();
  });

  it("removeCookie deletes cookie", () => {
    setCookie("to_remove", "value", 1);
    expect(getCookie("to_remove")).toBe("value");
    removeCookie("to_remove");
    expect(getCookie("to_remove")).toBeNull();
  });

  it("handles special characters in value", () => {
    setCookie("encoded", "hello world&foo=bar", 1);
    expect(getCookie("encoded")).toBe("hello world&foo=bar");
  });
});
