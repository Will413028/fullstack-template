import { apiClient } from "@/lib/api-client";
import type { LoginCredentials, TokenPair, UserResponse } from "../types";

export const authApi = {
  login(credentials: LoginCredentials): Promise<TokenPair> {
    const formData = new URLSearchParams();
    formData.append("username", credentials.account);
    formData.append("password", credentials.password);
    return apiClient.post<TokenPair>("/auth/login", formData);
  },

  register(credentials: LoginCredentials): Promise<TokenPair> {
    return apiClient.post<TokenPair>("/auth/register", credentials);
  },

  refresh(refreshToken: string): Promise<TokenPair> {
    return apiClient.post<TokenPair>("/auth/refresh", {
      refresh_token: refreshToken,
    });
  },

  logout(refreshToken: string): Promise<void> {
    return apiClient.post<void>("/auth/logout", {
      refresh_token: refreshToken,
    });
  },

  getMe(): Promise<{ data: UserResponse }> {
    return apiClient.get<{ data: UserResponse }>("/auth/me");
  },
};
