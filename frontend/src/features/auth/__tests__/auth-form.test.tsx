import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import messages from "../../../../messages/en.json";

const mockFetch = vi.fn<(request: Request) => Promise<Response>>();
vi.stubGlobal("fetch", mockFetch);

const push = vi.fn();

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/i18n/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  usePathname: () => "/",
  Link: ({ children }: React.PropsWithChildren<{ href: string }>) => children,
}));

const { AuthForm } = await import("../components/auth-form");

function problem(status: number, body: Record<string, unknown>) {
  return new Response(JSON.stringify({ status, title: "ignored", ...body }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

function renderForm(mode: "login" | "register") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  const result = render(
    <QueryClientProvider client={queryClient}>
      <NextIntlClientProvider locale="en" messages={messages}>
        <AuthForm mode={mode} />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );

  const form = result.container.querySelector("form");
  if (!form) throw new Error("Form not found");
  return within(form);
}

async function fillAndSubmit(
  form: ReturnType<typeof renderForm>,
  account: string,
  password: string,
) {
  const user = userEvent.setup();
  await user.type(form.getByLabelText(/account/i), account);
  await user.type(form.getByLabelText(/password/i), password);
  await user.click(form.getByRole("button", { name: /sign (in|up)/i }));
}

beforeEach(() => {
  mockFetch.mockReset();
  push.mockReset();
});

describe("AuthForm", () => {
  it("renders account and password fields", () => {
    const form = renderForm("login");
    expect(form.getByLabelText(/account/i)).toBeInTheDocument();
    expect(form.getByLabelText(/password/i)).toBeInTheDocument();
    expect(form.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
  });

  it("requires both fields without calling the API", async () => {
    const user = userEvent.setup();
    const form = renderForm("login");

    await user.click(form.getByRole("button", { name: /sign in/i }));

    expect(
      await screen.findAllByText(messages.problems.fields.required),
    ).toHaveLength(2);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("leaves length rules to the API", async () => {
    mockFetch.mockResolvedValueOnce(
      Response.json({ id: "u1", account: "newuser" }, { status: 201 }),
    );
    const form = renderForm("register");

    await fillAndSubmit(form, "newuser", "short");

    const [request] =
      mockFetch.mock.calls[0] ?? expect.unreachable("fetch was not called");
    expect(new URL(request.url).pathname).toBe("/api/users");
    expect(push).toHaveBeenCalledWith("/overview");
  });

  it("shows a field error from the API next to its field, translated", async () => {
    mockFetch.mockResolvedValueOnce(
      problem(422, {
        type: "/problems/validation",
        errors: [
          {
            pointer: "/password",
            code: "password_too_common",
            detail: "password is on the list of common or breached passwords",
          },
        ],
      }),
    );
    const form = renderForm("register");

    await fillAndSubmit(form, "newuser", "passwordpassword");

    expect(
      await screen.findByText(messages.problems.fields.password_too_common),
    ).toBeInTheDocument();
    expect(screen.queryByText(/on the list/)).not.toBeInTheDocument();
    expect(form.getByLabelText(/password/i)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("shows a wrong password as a translated message, not the detail", async () => {
    mockFetch.mockResolvedValueOnce(
      problem(401, {
        type: "/problems/unauthenticated",
        code: "invalid_credentials",
        detail: "account or password is incorrect",
      }),
    );
    const form = renderForm("login");

    await fillAndSubmit(form, "someone", "wrong-password");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      messages.problems.codes.invalid_credentials,
    );
    expect(screen.queryByText(/is incorrect/)).not.toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });
});
