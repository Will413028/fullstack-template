import {
  create,
  type DescMessage,
  type MessageInitShape,
  toBinary,
  toJson,
} from "@bufbuild/protobuf";
import { base64Encode } from "@bufbuild/protobuf/wire";
import { Code } from "@connectrpc/connect";
import {
  ErrorReason,
  FieldViolationReason,
} from "@/generated/app/common/v1/errors_pb";
import {
  BadRequestSchema,
  ErrorInfoSchema,
} from "@/generated/google/rpc/error_details_pb";
import { codeName } from "../connect-errors";

// Responses as the backend's connect-go writes them (https://connectrpc.com/docs/protocol/), for tests that stub fetch.
// The messages are built from the generated schemas, so a test cannot send a field or a reason the contract lacks.

// connectResponse is a successful unary response in JSON.
export function connectResponse<Desc extends DescMessage>(
  schema: Desc,
  init: MessageInitShape<Desc>,
): Response {
  return Response.json(toJson(schema, create(schema, init)));
}

// The HTTP status the protocol gives each code; the client reads the code from the body.
const httpStatus: Record<Code, number> = {
  [Code.Canceled]: 499,
  [Code.Unknown]: 500,
  [Code.InvalidArgument]: 400,
  [Code.DeadlineExceeded]: 504,
  [Code.NotFound]: 404,
  [Code.AlreadyExists]: 409,
  [Code.PermissionDenied]: 403,
  [Code.ResourceExhausted]: 429,
  [Code.FailedPrecondition]: 400,
  [Code.Aborted]: 409,
  [Code.OutOfRange]: 400,
  [Code.Unimplemented]: 501,
  [Code.Internal]: 500,
  [Code.Unavailable]: 503,
  [Code.DataLoss]: 500,
  [Code.Unauthenticated]: 401,
};

type Details = {
  // ErrorInfo.reason
  reason?: ErrorReason;
  // BadRequest's field violations
  violations?: { field: string; reason: FieldViolationReason }[];
};

// connectError is a unary error response: {code, message, details}, each detail its type name and its binary
// encoding in unpadded base64, as connect-go sends it.
export function connectError(
  code: Code,
  message: string,
  { reason, violations }: Details = {},
): Response {
  const details: { type: string; value: string }[] = [];
  if (reason !== undefined) {
    const info = create(ErrorInfoSchema, {
      reason: ErrorReason[reason],
      domain: "app",
    });
    details.push({
      type: ErrorInfoSchema.typeName,
      value: base64Encode(toBinary(ErrorInfoSchema, info), "std_raw"),
    });
  }
  if (violations !== undefined) {
    const badRequest = create(BadRequestSchema, {
      fieldViolations: violations.map((v) => ({
        field: v.field,
        reason: FieldViolationReason[v.reason],
      })),
    });
    details.push({
      type: BadRequestSchema.typeName,
      value: base64Encode(toBinary(BadRequestSchema, badRequest), "std_raw"),
    });
  }
  return Response.json(
    { code: codeName(code), message, details },
    { status: httpStatus[code] },
  );
}

// requestOf is what a stubbed fetch received: connect-web calls fetch(url, init), a JSON body as bytes.
export function requestOf(call: Parameters<typeof fetch> | undefined) {
  if (!call) throw new Error("fetch was not called");
  const [input, init = {}] = call;
  const { body } = init;
  return {
    url: new URL(String(input)),
    method: init.method,
    headers: new Headers(init.headers),
    body: ArrayBuffer.isView(body) ? new TextDecoder().decode(body) : body,
    cache: init.cache,
  };
}
