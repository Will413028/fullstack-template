import { Code } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorReason } from "@/generated/app/common/v1/errors_pb";
import { CreateSessionResponseSchema } from "@/generated/app/sessions/v1/sessions_pb";
import { DeleteCurrentUserResponseSchema } from "@/generated/app/users/v1/users_pb";
import {
  connectError,
  connectResponse,
  requestOf,
} from "@/lib/api/__tests__/connect-wire";
import messages from "../../../../messages/en.json";

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const replace = vi.fn();

vi.mock("@/i18n/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
}));

const { DeleteAccount } = await import("../components/delete-account");
const { transport } = await import("@/lib/api/transport");

function renderDelete() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  render(
    <TransportProvider transport={transport}>
      <QueryClientProvider client={queryClient}>
        <NextIntlClientProvider locale="en" messages={messages}>
          <DeleteAccount account="alice" />
        </NextIntlClientProvider>
      </QueryClientProvider>
    </TransportProvider>,
  );
}

async function confirmWith(password: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText(/password/i), password);
  await user.click(screen.getByRole("button", { name: /delete my account/i }));
}

// The transport's interceptor leaves the page by setting location.href.
const originalLocation = window.location;

beforeEach(() => {
  mockFetch.mockReset();
  replace.mockReset();
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

describe("DeleteAccount", () => {
  it("signs in again, deletes the account and leaves for the login page", async () => {
    mockFetch
      .mockResolvedValueOnce(
        connectResponse(CreateSessionResponseSchema, {
          session: { user: { account: "alice" } },
        }),
      )
      .mockResolvedValueOnce(
        connectResponse(DeleteCurrentUserResponseSchema, {}),
      );
    renderDelete();

    await confirmWith("the password");

    await vi.waitFor(() => expect(replace).toHaveBeenCalledWith("/login"));
    const signIn = requestOf(mockFetch.mock.calls[0]);
    expect(signIn.method).toBe("POST");
    expect(signIn.url.pathname).toBe(
      "/api/app.sessions.v1.SessionPublicService/CreateSession",
    );
    expect(JSON.parse(String(signIn.body))).toEqual({
      account: "alice",
      password: "the password",
    });
    const remove = requestOf(mockFetch.mock.calls[1]);
    expect(remove.method).toBe("POST");
    expect(remove.url.pathname).toBe(
      "/api/app.users.v1.UserAppService/DeleteCurrentUser",
    );
  });

  it("shows the error and deletes nothing when the password is wrong", async () => {
    mockFetch.mockResolvedValueOnce(
      connectError(Code.Unauthenticated, "account or password is incorrect", {
        reason: ErrorReason.INVALID_CREDENTIALS,
      }),
    );
    renderDelete();

    await confirmWith("wrong");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      messages.errors.reasons.INVALID_CREDENTIALS,
    );
    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(replace).not.toHaveBeenCalled();
    expect(window.location.href).toBe("");
  });
});
