import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { authApi, type Credentials, type User } from "../api/auth-api";

// The session lives in an HttpOnly cookie set by the backend — these hooks
// only deal with the user object and React Query cache.

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: () => authApi.getMe(),
    retry: false,
  });
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: Credentials) => authApi.login(credentials),
    onSuccess: (user) => {
      queryClient.setQueryData<User>(["auth", "me"], user);
    },
  });
}

export function useRegister() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: Credentials) => authApi.register(credentials),
    onSuccess: (user) => {
      queryClient.setQueryData<User>(["auth", "me"], user);
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
