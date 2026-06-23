import { apiClient } from "@/lib/api-client";
import type { LoginCredentials, UserResponse } from "../types";

// Auth tokens are delivered as httpOnly cookies by the backend; these calls
// only return the user. The browser stores/sends the cookies automatically.
export const authApi = {
  login(credentials: LoginCredentials): Promise<{ data: UserResponse }> {
    const formData = new URLSearchParams();
    formData.append("username", credentials.account);
    formData.append("password", credentials.password);
    return apiClient.post<{ data: UserResponse }>("/auth/login", formData);
  },

  register(credentials: LoginCredentials): Promise<{ data: UserResponse }> {
    return apiClient.post<{ data: UserResponse }>(
      "/auth/register",
      credentials,
    );
  },

  refresh(): Promise<{ data: UserResponse }> {
    return apiClient.post<{ data: UserResponse }>("/auth/refresh");
  },

  logout(): Promise<void> {
    return apiClient.post<void>("/auth/logout");
  },

  getMe(): Promise<{ data: UserResponse }> {
    return apiClient.get<{ data: UserResponse }>("/auth/me");
  },
};
