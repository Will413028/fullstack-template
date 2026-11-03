import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { ErrorReason } from "@/generated/app/common/v1/errors_pb";
import {
  BadRequestSchema,
  ErrorInfoSchema,
} from "@/generated/google/rpc/error_details_pb";
import { errorReason, fieldViolations } from "../connect-errors";
import { apiBaseUrl, redirectOnUnauthenticated } from "../transport";

function connectError(code: Code, reason?: ErrorReason) {
  const details =
    reason === undefined
      ? []
      : [
          {
            desc: ErrorInfoSchema,
            value: { reason: ErrorReason[reason], domain: "app" },
          },
        ];
  return new ConnectError("failed", code, undefined, details);
}

// Calls the interceptor around a call that fails with err.
function callFailing(err: unknown) {
  const next = () => Promise.reject(err);
  // the interceptor reads nothing from the request
  return redirectOnUnauthenticated(next as never)({} as never);
}

const originalLocation = window.location;

beforeEach(() => {
  Object.defineProperty(window, "location", {
    writable: true,
    value: { href: "", origin: originalLocation.origin },
  });
});

afterEach(() => {
  Object.defineProperty(window, "location", {
    writable: true,
    value: originalLocation,
  });
});

describe("Connect transport", () => {
  it("calls the API on the page's own origin under /api", () => {
    expect(apiBaseUrl()).toBe(`${originalLocation.origin}/api`);
  });

  it("sends the user to sign in when the session is gone", async () => {
    const err = connectError(Code.Unauthenticated, ErrorReason.SESSION_INVALID);
    await expect(callFailing(err)).rejects.toBe(err);
    expect(window.location.href).toBe("/login");
  });

  it("leaves a failed sign-in on its form", async () => {
    const err = connectError(
      Code.Unauthenticated,
      ErrorReason.INVALID_CREDENTIALS,
    );
    await expect(callFailing(err)).rejects.toBe(err);
    expect(window.location.href).toBe("");
  });

  it("does not redirect for other codes", async () => {
    const err = connectError(Code.PermissionDenied);
    await expect(callFailing(err)).rejects.toBe(err);
    expect(window.location.href).toBe("");
  });
});

describe("Connect error details", () => {
  it("reads the ErrorInfo reason", () => {
    expect(
      errorReason(connectError(Code.AlreadyExists, ErrorReason.ACCOUNT_TAKEN)),
    ).toBe("ACCOUNT_TAKEN");
    expect(errorReason(connectError(Code.NotFound))).toBeUndefined();
    expect(errorReason(new Error("network down"))).toBeUndefined();
  });

  it("maps each invalid field to its reason", () => {
    const err = new ConnectError("invalid", Code.InvalidArgument, undefined, [
      {
        desc: BadRequestSchema,
        value: create(BadRequestSchema, {
          fieldViolations: [
            { field: "password", reason: "TOO_SHORT" },
            { field: "account", reason: "PATTERN" },
            { field: "password", reason: "PASSWORD_TOO_COMMON" },
          ],
        }),
      },
    ]);
    expect(fieldViolations(err)).toEqual({
      password: "TOO_SHORT",
      account: "PATTERN",
    });
  });
});
