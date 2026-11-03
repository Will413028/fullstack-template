import { create, createFileRegistry } from "@bufbuild/protobuf";
import {
  FileDescriptorProtoSchema,
  MethodOptions_IdempotencyLevel,
} from "@bufbuild/protobuf/wkt";
import { Code, createClient } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorReason } from "@/generated/app/common/v1/errors_pb";
import { connectError } from "./connect-wire";

// The transport captures fetch when it is created, so stub it first.
const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const { transport } = await import("../transport");

// A service shaped like the contract's: a read marked NO_SIDE_EFFECTS and a write. It is described here rather than
// taken from the contract, so these tests of the transport do not change with the contract's services.
const registry = createFileRegistry(
  create(FileDescriptorProtoSchema, {
    name: "app/probe/v1/probe.proto",
    package: "app.probe.v1",
    syntax: "proto3",
    messageType: [{ name: "Empty" }],
    service: [
      {
        name: "ProbeAppService",
        method: [
          {
            name: "GetProbe",
            inputType: ".app.probe.v1.Empty",
            outputType: ".app.probe.v1.Empty",
            options: {
              idempotencyLevel: MethodOptions_IdempotencyLevel.NO_SIDE_EFFECTS,
            },
          },
          {
            name: "UpdateProbe",
            inputType: ".app.probe.v1.Empty",
            outputType: ".app.probe.v1.Empty",
          },
        ],
      },
    ],
  }),
  () => undefined,
);
const service = registry.getService("app.probe.v1.ProbeAppService");
if (!service) throw new Error("no ProbeAppService");
// A service from a registry has no generated types; both methods are unary.
const { getProbe, updateProbe } = createClient(
  service,
  transport,
) as unknown as Record<
  "getProbe" | "updateProbe",
  (req: object) => Promise<unknown>
>;

beforeEach(() => {
  mockFetch.mockReset();
  mockFetch.mockResolvedValue(Response.json({}));
});

function sent() {
  const [input, init] =
    mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
  return { url: new URL(String(input)), init: init ?? {} };
}

it("calls reads with GET on the page's origin under /api, with a timeout", async () => {
  await getProbe({});

  const { url, init } = sent();
  expect(init.method).toBe("GET");
  expect(url.origin + url.pathname).toBe(
    `${window.location.origin}/api/app.probe.v1.ProbeAppService/GetProbe`,
  );
  expect(new Headers(init.headers).get("connect-timeout-ms")).toBe("15000");
});

it("calls writes with POST, which the server's CSRF check covers", async () => {
  await updateProbe({});

  const { url, init } = sent();
  expect(init.method).toBe("POST");
  expect(url.pathname).toBe("/api/app.probe.v1.ProbeAppService/UpdateProbe");
});

describe("a failed call", () => {
  // The redirect interceptor leaves the page by setting location.href.
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

  it("sends the user to sign in when the session is gone, without trying again", async () => {
    mockFetch.mockResolvedValueOnce(
      connectError(Code.Unauthenticated, "session is invalid", {
        reason: ErrorReason.SESSION_INVALID,
      }),
    );

    await expect(getProbe({})).rejects.toMatchObject({
      code: Code.Unauthenticated,
    });

    expect(window.location.href).toBe("/login");
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("turns a response that is not a Connect error into the code of its status", async () => {
    // a proxy's error page, not the backend's
    mockFetch.mockResolvedValueOnce(
      new Response("<html>Bad Gateway</html>", {
        status: 502,
        statusText: "Bad Gateway",
        headers: { "Content-Type": "text/html" },
      }),
    );

    await expect(updateProbe({})).rejects.toMatchObject({
      code: Code.Unavailable,
    });
    expect(window.location.href).toBe("");
  });
});
