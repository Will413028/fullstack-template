import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getCookie, removeCookie, setCookie } from "@/lib/cookies";
import { authApi } from "../api/auth-api";
import type { LoginCredentials, TokenPair } from "../types";

function storeTokens(tokens: TokenPair) {
  setCookie("auth_token", tokens.access_token, 1); // 1 day (access token is short-lived)
  setCookie("refresh_token", tokens.refresh_token, 7); // 7 days
}

function clearTokens() {
  removeCookie("auth_token");
  removeCookie("refresh_token");
}

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: () => authApi.getMe(),
    retry: false,
    enabled: !!getCookie("auth_token"),
  });
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: LoginCredentials) => authApi.login(credentials),
    onSuccess: (data) => {
      storeTokens(data);
      queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });
}

export function useRegister() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (credentials: LoginCredentials) =>
      authApi.register(credentials),
    onSuccess: (data) => {
      storeTokens(data);
      queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => {
      const refreshToken = getCookie("refresh_token");
      if (refreshToken) {
        return authApi.logout(refreshToken);
      }
      return Promise.resolve();
    },
    onSettled: () => {
      clearTokens();
      queryClient.clear();
    },
  });
}

export function useRefreshToken() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => {
      const refreshToken = getCookie("refresh_token");
      if (!refreshToken) throw new Error("No refresh token");
      return authApi.refresh(refreshToken);
    },
    onSuccess: (data) => {
      storeTokens(data);
      queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    },
    onError: () => {
      clearTokens();
      queryClient.clear();
    },
  });
}
