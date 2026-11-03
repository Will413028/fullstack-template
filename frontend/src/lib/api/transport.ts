import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ErrorReason } from "@/generated/app/common/v1/errors_pb";
import { apiBaseUrl } from "./base-url";
import { errorReason } from "./connect-errors";

// The Connect transport for the API in ../api (docs/adr/2026-11-03-api-contract-protobuf-connect.md), at the same
// /api as the REST client in client.ts, which it replaces as each service moves to Connect.

// How long a call may take before the client gives up; Connect sends it as Connect-Timeout-Ms and the server's
// context ends with it.
const defaultTimeoutMs = 15_000;

// unauthenticated means the session is gone. Sessions are opaque and never refreshed, so the only remedy is to sign
// in again. Signing in itself answers unauthenticated with INVALID_CREDENTIALS for a wrong password, which the form
// shows instead: the reason decides, not a list of procedures.
export const redirectOnUnauthenticated: Interceptor = (next) => async (req) => {
  try {
    return await next(req);
  } catch (err) {
    const ce = ConnectError.from(err);
    if (
      ce.code === Code.Unauthenticated &&
      typeof window !== "undefined" &&
      errorReason(ce) !== ErrorReason[ErrorReason.INVALID_CREDENTIALS]
    ) {
      window.location.href = "/login";
    }
    throw err;
  }
};

// Reads (methods marked NO_SIDE_EFFECTS) go out as GET; everything else is a POST, which CrossOriginProtection on
// the server checks.
export const transport = createConnectTransport({
  baseUrl: apiBaseUrl(),
  useHttpGet: true,
  defaultTimeoutMs,
  interceptors: [redirectOnUnauthenticated],
});
