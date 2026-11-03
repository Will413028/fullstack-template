import { create } from "@bufbuild/protobuf";
import {
  createConnectQueryKey,
  useQuery,
  useTransport,
} from "@connectrpc/connect-query";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  GetCurrentUserResponseSchema,
  type User,
  UserAppService,
} from "@/generated/app/users/v1/users_pb";
import { authApi, type Credentials } from "../api/auth-api";

// The session lives in an HttpOnly cookie set by the backend — these hooks
// only deal with the user object and React Query cache. The current user is
// connect-query's GetCurrentUser query; the transport comes from the
// TransportProvider (providers/query-provider.tsx), the same one authApi uses.

const getCurrentUser = UserAppService.method.getCurrentUser;

export function useCurrentUser() {
  return useQuery(
    getCurrentUser,
    {},
    { retry: false, select: (response) => response.user },
  );
}

// After signing in or up, the current user is the account just signed in to:
// set it instead of fetching it again. The key is the one useCurrentUser's
// query has (same method, input and transport).
function useSetCurrentUser() {
  const queryClient = useQueryClient();
  const transport = useTransport();

  return (user: User) => {
    queryClient.setQueryData(
      createConnectQueryKey({
        schema: getCurrentUser,
        input: {},
        transport,
        cardinality: "finite",
      }),
      create(GetCurrentUserResponseSchema, { user }),
    );
  };
}

export function useLogin() {
  const setCurrentUser = useSetCurrentUser();

  return useMutation({
    mutationFn: (credentials: Credentials) => authApi.login(credentials),
    onSuccess: setCurrentUser,
  });
}

export function useRegister() {
  const setCurrentUser = useSetCurrentUser();

  return useMutation({
    mutationFn: (credentials: Credentials) => authApi.register(credentials),
    onSuccess: setCurrentUser,
  });
}

export function useDeleteAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: Credentials) =>
      authApi.deleteAccount(credentials),
    onSuccess: () => {
      queryClient.clear();
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => authApi.logout(),
    onSettled: () => {
      queryClient.clear();
    },
  });
}
