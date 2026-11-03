import { Code } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";
import {
  ErrorReason,
  FieldViolationReason,
} from "@/generated/app/common/v1/errors_pb";
import en from "../../../../messages/en.json";
import zhTW from "../../../../messages/zh-TW.json";
import {
  type CodeName,
  codeName,
  type ErrorReasonName,
  type FieldViolationReasonName,
} from "../connect-errors";

// Compile-time half (pnpm lint): every Connect code, every ErrorReason and every FieldViolationReason the contract
// declares has a message in every locale. A value added to api/app/common/v1/errors.proto without a translation fails
// tsc here instead of showing a vaguer message.
type Covers<Key extends string> = Record<Key, string>;
type Errors = {
  codes: Covers<CodeName>;
  reasons: Covers<ErrorReasonName>;
  fields: Covers<FieldViolationReasonName>;
};
const locales: Record<string, Errors> = { en: en.errors, zhTW: zhTW.errors };

// The names the runtime has (an enum object also maps each number back to its name), without *_UNSPECIFIED.
function names(enumObject: object, unspecified: string): string[] {
  return Object.keys(enumObject)
    .filter((key) => Number.isNaN(Number(key)) && key !== unspecified)
    .sort();
}

describe("error messages", () => {
  it.each(Object.entries(locales))(
    "%s translates exactly the contract's codes and reasons",
    (_, errors) => {
      const codes = Object.values(Code)
        .filter((code): code is Code => typeof code === "number")
        .map(codeName)
        .sort();
      expect(Object.keys(errors.codes).sort()).toEqual(codes);
      expect(Object.keys(errors.reasons).sort()).toEqual(
        names(ErrorReason, "ERROR_REASON_UNSPECIFIED"),
      );
      expect(Object.keys(errors.fields).sort()).toEqual(
        names(FieldViolationReason, "FIELD_VIOLATION_REASON_UNSPECIFIED"),
      );
    },
  );
});
