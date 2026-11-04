import { Code } from "@connectrpc/connect";
import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ErrorReason,
  FieldViolationReason,
} from "@/generated/app/common/v1/errors_pb";
import { CreateUserResponseSchema } from "@/generated/app/users/v1/users_pb";
import {
  connectError,
  connectResponse,
  requestOf,
} from "@/lib/api/__tests__/connect-wire";
import messages from "../../../../messages/en.json";

const mockFetch = vi.fn<typeof fetch>();
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
const { transport } = await import("@/lib/api/transport");

function renderForm(mode: "login" | "register") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  const result = render(
    <TransportProvider transport={transport}>
      <QueryClientProvider client={queryClient}>
        <NextIntlClientProvider locale="en" messages={messages}>
          <AuthForm mode={mode} />
        </NextIntlClientProvider>
      </QueryClientProvider>
    </TransportProvider>,
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

// The transport's interceptor leaves the page by setting location.href.
const originalLocation = window.location;

beforeEach(() => {
  mockFetch.mockReset();
  push.mockReset();
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
      await screen.findAllByText(messages.errors.fields.REQUIRED),
    ).toHaveLength(2);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("leaves length rules to the API", async () => {
    mockFetch.mockResolvedValueOnce(
      connectResponse(CreateUserResponseSchema, {
        user: { id: "u1", account: "newuser" },
      }),
    );
    const form = renderForm("register");

    await fillAndSubmit(form, "newuser", "short");

    const request = requestOf(mockFetch.mock.calls[0]);
    expect(request.method).toBe("POST");
    expect(request.url.pathname).toBe(
      "/api/app.users.v1.UserPublicService/CreateUser",
    );
    expect(request.headers.get("Content-Type")).toBe("application/json");
    expect(JSON.parse(String(request.body))).toEqual({
      account: "newuser",
      password: "short",
    });
    expect(push).toHaveBeenCalledWith("/overview");
  });

  it("shows a field error from the API next to its field, translated", async () => {
    mockFetch.mockResolvedValueOnce(
      connectError(
        Code.InvalidArgument,
        "password is on the list of common or breached passwords",
        {
          violations: [
            {
              field: "password",
              reason: FieldViolationReason.PASSWORD_TOO_COMMON,
            },
          ],
        },
      ),
    );
    const form = renderForm("register");

    await fillAndSubmit(form, "newuser", "passwordpassword");

    expect(
      await screen.findByText(messages.errors.fields.PASSWORD_TOO_COMMON),
    ).toBeInTheDocument();
    expect(screen.queryByText(/on the list/)).not.toBeInTheDocument();
    expect(form.getByLabelText(/password/i)).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    // the field carries the error, so the form adds no general message
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("sign-in failure stays on the login page and shows the error", async () => {
    mockFetch.mockResolvedValueOnce(
      connectError(Code.Unauthenticated, "account or password is incorrect", {
        reason: ErrorReason.INVALID_CREDENTIALS,
      }),
    );
    const form = renderForm("login");

    await fillAndSubmit(form, "someone", "wrong-password");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      messages.errors.reasons.INVALID_CREDENTIALS,
    );
    expect(screen.queryByText(/is incorrect/)).not.toBeInTheDocument();
    expect(requestOf(mockFetch.mock.calls[0]).url.pathname).toBe(
      "/api/app.sessions.v1.SessionPublicService/CreateSession",
    );
    expect(window.location.href).toBe("");
    expect(push).not.toHaveBeenCalled();
  });
});
