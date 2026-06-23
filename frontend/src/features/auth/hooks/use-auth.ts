import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { authApi } from "../api/auth-api";
import type { LoginCredentials, UserResponse } from "../types";

// Tokens live in httpOnly cookies set by the backend — these hooks only deal
// with the user object and React Query cache; they never read/write tokens.

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: () => authApi.getMe().then((r) => r.data),
    retry: false,
  });
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: LoginCredentials) => authApi.login(credentials),
    onSuccess: (res) => {
      queryClient.setQueryData<UserResponse>(["auth", "me"], res.data);
    },
  });
}

export function useRegister() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: LoginCredentials) =>
      authApi.register(credentials),
    onSuccess: (res) => {
      queryClient.setQueryData<UserResponse>(["auth", "me"], res.data);
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

export function useRefreshToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => authApi.refresh(),
    onSuccess: (res) => {
      queryClient.setQueryData<UserResponse>(["auth", "me"], res.data);
    },
    onError: () => {
      queryClient.clear();
    },
  });
}
