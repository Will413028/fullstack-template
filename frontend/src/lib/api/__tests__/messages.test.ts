import { describe, expect, it } from "vitest";
import en from "../../../../messages/en.json";
import zhTW from "../../../../messages/zh-TW.json";
import type { FieldErrorCode, ProblemCode, ProblemTypeSlug } from "../errors";

// Compile-time half (pnpm lint): every problem type, code and field error code
// the contract declares has a message. A value added to openapi.yaml without a
// translation fails tsc here instead of showing a vaguer message.
type Covers<Key extends string> = Record<Key, string>;
type Problems = {
  types: Covers<ProblemTypeSlug>;
  codes: Covers<ProblemCode>;
  fields: Covers<FieldErrorCode>;
};
const enProblems: Problems = en.problems;
const zhTWProblems: Problems = zhTW.problems;

describe("problem messages", () => {
  it("translates the contract's values in every locale", () => {
    expect(Object.keys(zhTWProblems.types).sort()).toEqual(
      Object.keys(enProblems.types).sort(),
    );
    expect(Object.keys(zhTWProblems.codes).sort()).toEqual(
      Object.keys(enProblems.codes).sort(),
    );
    expect(Object.keys(zhTWProblems.fields).sort()).toEqual(
      Object.keys(enProblems.fields).sort(),
    );
  });
});
