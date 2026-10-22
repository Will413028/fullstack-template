import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import messages from "../../../../messages/en.json";

const mockFetch = vi.fn<(request: Request) => Promise<Response>>();
vi.stubGlobal("fetch", mockFetch);

const replace = vi.fn();

vi.mock("@/i18n/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
}));

const { DeleteAccount } = await import("../components/delete-account");

function renderDelete() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <NextIntlClientProvider locale="en" messages={messages}>
        <DeleteAccount account="alice" />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );
}

async function confirmWith(password: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText(/password/i), password);
  await user.click(screen.getByRole("button", { name: /delete my account/i }));
}

beforeEach(() => {
  mockFetch.mockReset();
  replace.mockReset();
});

describe("DeleteAccount", () => {
  it("signs in again, deletes the account and leaves for the login page", async () => {
    mockFetch
      .mockResolvedValueOnce(Response.json({ user: {} }, { status: 201 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    renderDelete();

    await confirmWith("the password");

    await vi.waitFor(() => expect(replace).toHaveBeenCalledWith("/login"));
    const [signIn] =
      mockFetch.mock.calls[0] ?? expect.unreachable("no sign-in");
    expect(new URL(signIn.url).pathname).toBe("/api/sessions");
    expect(await signIn.json()).toEqual({
      account: "alice",
      password: "the password",
    });
    const [remove] = mockFetch.mock.calls[1] ?? expect.unreachable("no delete");
    expect(remove.method).toBe("DELETE");
    expect(new URL(remove.url).pathname).toBe("/api/users/me");
  });

  it("shows the problem and deletes nothing when the password is wrong", async () => {
    mockFetch.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          type: "/problems/unauthenticated",
          title: "Unauthenticated",
          status: 401,
          code: "invalid_credentials",
        }),
        {
          status: 401,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
    );
    renderDelete();

    await confirmWith("wrong");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      messages.problems.codes.invalid_credentials,
    );
    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(replace).not.toHaveBeenCalled();
  });
});
