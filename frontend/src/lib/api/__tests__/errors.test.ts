import { type AbstractIntlMessages, createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";
import en from "../../../../messages/en.json";
import {
  fieldErrors,
  fieldMessage,
  type Problem,
  type ProblemCode,
  problemMessage,
} from "../errors";

const t = createTranslator({
  locale: "en",
  // untyped, like useTranslations without a global AppConfig
  messages: en as AbstractIntlMessages,
  namespace: "problems",
});

const base = { title: "ignored", status: 400 };

describe("problemMessage", () => {
  it("translates the code", () => {
    const problem: Problem = {
      ...base,
      type: "/problems/unauthenticated",
      code: "invalid_credentials",
      detail: "account or password is incorrect",
    };
    expect(problemMessage(t, problem)).toBe(
      en.problems.codes.invalid_credentials,
    );
  });

  it("falls back to the type when the code has no message", () => {
    const problem: Problem = {
      ...base,
      type: "/problems/rate-limited",
      // a code from a newer server than this client
      code: "something_new" as ProblemCode,
      detail: "slow down",
    };
    expect(problemMessage(t, problem)).toBe(en.problems.types["rate-limited"]);
  });

  it("never shows detail, even for an unknown type", () => {
    const problem: Problem = {
      ...base,
      type: "about:blank",
      detail: "raw detail",
    };
    expect(problemMessage(t, problem)).toBe(en.problems.unknown);
  });
});

describe("fieldErrors and fieldMessage", () => {
  it("keys errors by body property and translates their codes", () => {
    const problem: Problem = {
      ...base,
      type: "/problems/validation",
      errors: [
        {
          pointer: "/password",
          code: "password_too_common",
          detail: "on the list",
        },
        { pointer: "/account", code: "pattern", detail: "bad" },
        { pointer: "", code: "required", detail: "no body" },
      ],
    };

    const fields = fieldErrors(problem);
    expect([...fields.keys()]).toEqual(["password", "account"]);
    expect(fieldMessage(t, "password_too_common")).toBe(
      en.problems.fields.password_too_common,
    );
    expect(fieldMessage(t, "not_a_code")).toBe(en.problems.fields.invalid);
  });
});
