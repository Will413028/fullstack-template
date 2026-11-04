import { TransportProvider } from "@connectrpc/connect-query";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateSessionResponseSchema } from "@/generated/app/sessions/v1/sessions_pb";
import {
  CreateUserResponseSchema,
  GetCurrentUserResponseSchema,
} from "@/generated/app/users/v1/users_pb";
import { connectResponse, requestOf } from "@/lib/api/__tests__/connect-wire";

const mockFetch = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", mockFetch);

const { useCurrentUser, useLogin, useRegister } = await import(
  "../hooks/use-auth"
);
const { transport } = await import("@/lib/api/transport");

const paths = {
  me: "/api/app.users.v1.UserAppService/GetCurrentUser",
  login: "/api/app.sessions.v1.SessionPublicService/CreateSession",
  register: "/api/app.users.v1.UserPublicService/CreateUser",
};

// Answers by procedure, so the order of the calls does not matter: the
// current user is alice until bob signs in or up.
function serve(input: Parameters<typeof fetch>[0]): Promise<Response> {
  const { pathname } = new URL(String(input));
  if (pathname === paths.me) {
    return Promise.resolve(
      connectResponse(GetCurrentUserResponseSchema, {
        user: { account: "alice" },
      }),
    );
  }
  if (pathname === paths.login) {
    return Promise.resolve(
      connectResponse(CreateSessionResponseSchema, {
        session: { user: { account: "bob" } },
      }),
    );
  }
  if (pathname === paths.register) {
    return Promise.resolve(
      connectResponse(CreateUserResponseSchema, { user: { account: "bob" } }),
    );
  }
  return Promise.reject(new Error(`unexpected call to ${pathname}`));
}

function CurrentUser({ mode }: { mode: "login" | "register" }) {
  const me = useCurrentUser();
  const login = useLogin();
  const register = useRegister();
  const submit = mode === "login" ? login : register;
  return (
    <>
      <p>{me.data?.account ?? "loading"}</p>
      <button
        type="button"
        onClick={() => submit.mutate({ account: "bob", password: "pw" })}
      >
        go
      </button>
    </>
  );
}

beforeEach(() => {
  mockFetch.mockReset();
  mockFetch.mockImplementation(serve);
});

describe("auth hooks", () => {
  it.each(["login", "register"] as const)(
    "after %s the current user is the new account, without fetching it again",
    async (mode) => {
      const queryClient = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      });
      render(
        <TransportProvider transport={transport}>
          <QueryClientProvider client={queryClient}>
            <CurrentUser mode={mode} />
          </QueryClientProvider>
        </TransportProvider>,
      );
      expect(await screen.findByText("alice")).toBeInTheDocument();

      await userEvent.setup().click(screen.getByRole("button", { name: "go" }));

      expect(await screen.findByText("bob")).toBeInTheDocument();
      const called = mockFetch.mock.calls.map(
        (call) => requestOf(call).url.pathname,
      );
      expect(called.sort()).toEqual([paths.me, paths[mode]].sort());
    },
  );
});
