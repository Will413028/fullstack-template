import "server-only";

import { createConnectTransport } from "@connectrpc/connect-web";
import { sharedTransportOptions } from "./transport";

// The transport for Server Components. A call passes the visitor's cookie itself (headers: { cookie }), the only
// header it forwards; the answer depends on that cookie, so Next's fetch must never cache it. There is no redirect
// interceptor: the server has no page to leave, and the caller decides what unauthenticated means.
export const serverTransport = createConnectTransport({
  ...sharedTransportOptions(),
  fetch: (input, init) => fetch(input, { ...init, cache: "no-store" }),
});
