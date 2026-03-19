import { apiClient } from "@/lib/api-client";
import type { AuthResponse, LoginCredentials } from "../types";

export const authApi = {
  login(credentials: LoginCredentials): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>("/auth/login", credentials);
  },

  logout(): Promise<void> {
    return apiClient.post<void>("/auth/logout");
  },

  getMe(): Promise<AuthResponse["user"]> {
    return apiClient.get<AuthResponse["user"]>("/auth/me");
  },
};
