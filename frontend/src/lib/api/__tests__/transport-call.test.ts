import { create, createFileRegistry } from "@bufbuild/protobuf";
import {
  FileDescriptorProtoSchema,
  MethodOptions_IdempotencyLevel,
} from "@bufbuild/protobuf/wkt";
import { createClient } from "@connectrpc/connect";
import { beforeEach, expect, it, vi } from "vitest";

// The transport captures fetch when it is created, so stub it first.
const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const { transport } = await import("../transport");

// A service shaped like the contract's: a read marked NO_SIDE_EFFECTS and a write. The contract has no service the
// frontend calls yet (docs/plans/2026-11-03-connect-contract.md step 1), so this one is described here.
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
