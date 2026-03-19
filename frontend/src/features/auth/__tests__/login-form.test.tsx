import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/i18n/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/",
  Link: ({ children }: React.PropsWithChildren<{ href: string }>) => children,
}));

const { LoginForm } = await import("../components/login-form");

const messages = {
  auth: {
    login: "Sign In",
    email: "Email",
    password: "Password",
  },
};

function renderLoginForm() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  const result = render(
    <QueryClientProvider client={queryClient}>
      <NextIntlClientProvider locale="en" messages={messages}>
        <LoginForm />
      </NextIntlClientProvider>
    </QueryClientProvider>,
  );

  const form = result.container.querySelector("form");
  if (!form) throw new Error("Form not found");
  return { ...result, form: within(form) };
}

describe("LoginForm", () => {
  it("renders account and password fields", () => {
    const { form } = renderLoginForm();
    expect(form.getByLabelText(/email/i)).toBeInTheDocument();
    expect(form.getByLabelText(/password/i)).toBeInTheDocument();
    expect(form.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
  });

  it("shows validation error for empty account", async () => {
    const user = userEvent.setup();
    const { form } = renderLoginForm();

    await user.click(form.getByRole("button", { name: /sign in/i }));
    expect(await screen.findByText(/account is required/i)).toBeInTheDocument();
  });

  it("shows validation error for short password", async () => {
    const user = userEvent.setup();
    const { form } = renderLoginForm();

    await user.type(form.getByPlaceholderText("your_account"), "testuser");
    await user.type(form.getByPlaceholderText("Enter your password"), "short");
    await user.click(form.getByRole("button", { name: /sign in/i }));
    const errors = await screen.findAllByText(/at least 8 characters/i);
    expect(errors.length).toBeGreaterThan(0);
  });
});
