import { Code, ConnectError } from "@connectrpc/connect";
import type {
  ErrorReason,
  FieldViolationReason,
} from "@/generated/app/common/v1/errors_pb";
import {
  BadRequestSchema,
  ErrorInfoSchema,
} from "@/generated/google/rpc/error_details_pb";

// A Connect error is a code plus google.rpc details: ErrorInfo.reason is an app.common.v1.ErrorReason value when the
// client needs to tell failures apart, BadRequest lists invalid fields with a FieldViolationReason value each. The
// message is for developers: what the user reads comes from the code and the reasons (decision D3), translated in
// messages/*.json under errors.codes, errors.reasons and errors.fields. A reason this client does not know falls back
// to the code.

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

// The names messages/*.json translates. The value names are the strings on the wire; *_UNSPECIFIED is never sent.
// messages.test.ts checks at compile time that every locale has each of them, so a value added to the contract
// without a translation fails pnpm lint.
type SnakeCase<S extends string> = S extends `${infer C}${infer Rest}`
  ? `${C extends Uppercase<C> ? `_${Lowercase<C>}` : C}${SnakeCase<Rest>}`
  : "";
export type CodeName = SnakeCase<Uncapitalize<keyof typeof Code>>;
export type ErrorReasonName = Exclude<
  keyof typeof ErrorReason,
  "ERROR_REASON_UNSPECIFIED"
>;
export type FieldViolationReasonName = Exclude<
  keyof typeof FieldViolationReason,
  "FIELD_VIOLATION_REASON_UNSPECIFIED"
>;

// codeName is the code's name on the Connect wire (snake_case, e.g. invalid_argument), the key of errors.codes.
export function codeName(code: Code): CodeName {
  const name = Code[code] ?? Code[Code.Unknown];
  return name
    .replace(/^[A-Z]/, (c) => c.toLowerCase())
    .replace(/[A-Z]/g, (c) => `_${c.toLowerCase()}`) as CodeName;
}

// next-intl's translator for the "errors" namespace.
type Translate = ((key: string) => string) & { has(key: string): boolean };

// errorMessage is what the user reads for a failed call: the ErrorInfo reason when this client translates it, else
// the code. Anything that is not a Connect error (a bug in this client) is the code unknown.
export function errorMessage(t: Translate, err: unknown): string {
  const ce = ConnectError.from(err);
  const reason = errorReason(ce);
  if (reason && t.has(`reasons.${reason}`)) return t(`reasons.${reason}`);
  return t(`codes.${codeName(ce.code)}`);
}

const invalidField: FieldViolationReasonName = "INVALID";

// violationMessage translates a field violation's reason, from the API or the form's own presence check; a reason
// this client does not know reads as INVALID.
export function violationMessage(t: Translate, reason: string): string {
  const key = `fields.${reason}`;
  return t.has(key) ? t(key) : t(`fields.${invalidField}`);
}
