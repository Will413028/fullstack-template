import { Code, ConnectError } from "@connectrpc/connect";
import { type AbstractIntlMessages, createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";
import { ErrorInfoSchema } from "@/generated/google/rpc/error_details_pb";
import en from "../../../../messages/en.json";
import { codeName, errorMessage, violationMessage } from "../connect-errors";

const t = createTranslator({
  locale: "en",
  // untyped, like useTranslations without a global AppConfig
  messages: en as AbstractIntlMessages,
  namespace: "errors",
});

function withReason(code: Code, reason: string, message: string) {
  return new ConnectError(message, code, undefined, [
    { desc: ErrorInfoSchema, value: { reason, domain: "app" } },
  ]);
}

describe("errorMessage", () => {
  it("translates the reason, not the message", () => {
    const err = withReason(
      Code.Unauthenticated,
      "INVALID_CREDENTIALS",
      "account or password is incorrect",
    );
    expect(errorMessage(t, err)).toBe(en.errors.reasons.INVALID_CREDENTIALS);
  });

  it("falls back to the code when the reason has no message", () => {
    // a reason from a newer server than this client
    const err = withReason(
      Code.ResourceExhausted,
      "SOMETHING_NEW",
      "slow down",
    );
    expect(errorMessage(t, err)).toBe(en.errors.codes.resource_exhausted);
    expect(errorMessage(t, new ConnectError("gone", Code.NotFound))).toBe(
      en.errors.codes.not_found,
    );
  });

  it("never shows the message, even for an error that is not a Connect error", () => {
    expect(errorMessage(t, new Error("raw detail"))).toBe(
      en.errors.codes.unknown,
    );
  });
});

describe("violationMessage", () => {
  it("translates the reason, and one this client does not know as INVALID", () => {
    expect(violationMessage(t, "PASSWORD_TOO_COMMON")).toBe(
      en.errors.fields.PASSWORD_TOO_COMMON,
    );
    expect(violationMessage(t, "NOT_A_REASON")).toBe(en.errors.fields.INVALID);
  });
});

describe("codeName", () => {
  it("names each code as the Connect protocol does on the wire", () => {
    const codes = Object.values(Code).filter(
      (code): code is Code => typeof code === "number",
    );
    // https://connectrpc.com/docs/protocol/#error-codes
    expect(codes.map(codeName)).toEqual([
      "canceled",
      "unknown",
      "invalid_argument",
      "deadline_exceeded",
      "not_found",
      "already_exists",
      "permission_denied",
      "resource_exhausted",
      "failed_precondition",
      "aborted",
      "out_of_range",
      "unimplemented",
      "internal",
      "unavailable",
      "data_loss",
      "unauthenticated",
    ]);
  });
});
