import { ConnectError } from "@connectrpc/connect";
import {
  BadRequestSchema,
  ErrorInfoSchema,
} from "@/generated/google/rpc/error_details_pb";

// A Connect error is a code plus google.rpc details: ErrorInfo.reason is an app.common.v1.ErrorReason value when the
// client needs to tell failures apart, BadRequest lists invalid fields with a FieldViolationReason value each. The
// message is for developers: what the user reads comes from the code and the reasons (decision D3), translated in
// messages/*.json. A reason this client does not know falls back to the code.

// errorReason is the ErrorInfo reason of err, undefined without one.
export function errorReason(err: unknown): string | undefined {
  return ConnectError.from(err).findDetails(ErrorInfoSchema)[0]?.reason;
}

// fieldViolations maps each invalid request field (its path, e.g. password or thing.name) to its reason.
export function fieldViolations(err: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  for (const br of ConnectError.from(err).findDetails(BadRequestSchema)) {
    for (const v of br.fieldViolations) {
      out[v.field] ??= v.reason;
    }
  }
  return out;
}
