import type { components } from "./schema";

type FieldError = components["schemas"]["FieldError"];
type ContractProblem = components["schemas"]["Problem"];

// The values the contract declares as enums; messages/*.json must translate
// each of them, which messages.test.ts checks at compile time.
export type ProblemTypeSlug =
  ContractProblem["type"] extends `/problems/${infer Slug}` ? Slug : never;
export type ProblemCode = NonNullable<ContractProblem["code"]>;
export type FieldErrorCode = FieldError["code"];

// An RFC 9457 problem as the backend sends it; validation problems add errors.
// about:blank is what unwrap makes of a response that is not a problem.
export type Problem = Omit<ContractProblem, "type"> & {
  type: ContractProblem["type"] | "about:blank";
  errors?: FieldError[];
};

export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem;

  constructor(status: number, problem: Problem) {
    super(problem.title);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

function isProblem(body: unknown): body is Problem {
  return (
    typeof body === "object" &&
    body !== null &&
    typeof (body as Problem).type === "string"
  );
}

// Returns the data of an openapi-fetch result, or throws an ApiError. A body
// that is not a problem (a proxy's error page, an empty 502) becomes one with
// type about:blank, which translates to the generic message.
export async function unwrap<T>(
  call: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  const { data, error, response } = await call;
  if (response.ok) return data as T;
  const problem: Problem = isProblem(error)
    ? error
    : {
        type: "about:blank",
        title: response.statusText,
        status: response.status,
      };
  throw new ApiError(response.status, problem);
}

// next-intl's translator for the "problems" namespace.
type Translate = ((key: string) => string) & { has(key: string): boolean };

const typePrefix = "/problems/";

// Messages are chosen by code, then by type (RFC 9457); detail is for developers
// and is never shown.
export function problemMessage(t: Translate, problem: Problem): string {
  if (problem.code && t.has(`codes.${problem.code}`)) {
    return t(`codes.${problem.code}`);
  }
  if (problem.type.startsWith(typePrefix)) {
    const key = `types.${problem.type.slice(typePrefix.length)}`;
    if (t.has(key)) return t(key);
  }
  return t("unknown");
}

// The message for a field error code, from the backend or the form's own
// presence check.
export function fieldMessage(t: Translate, code: string): string {
  const key = `fields.${code}`;
  return t.has(key) ? t(key) : t("fields.invalid");
}

// The field errors of a validation problem, keyed by top-level body property
// ("/password" → "password"). Errors without a pointer to a property are left
// out; the form shows problemMessage for them.
export function fieldErrors(problem: Problem): Map<string, FieldError> {
  const fields = new Map<string, FieldError>();
  for (const error of problem.errors ?? []) {
    const name = error.pointer.split("/")[1];
    if (name && !fields.has(name)) fields.set(name, error);
  }
  return fields;
}
