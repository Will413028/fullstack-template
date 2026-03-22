import { getCookie, removeCookie, setCookie } from "./cookies";

export class ApiError extends Error {
  status: number;
  errors?: Record<string, string[]>;

  constructor(
    status: number,
    message: string,
    errors?: Record<string, string[]>,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.errors = errors;
  }
}

type RequestOptions = Omit<RequestInit, "method" | "body"> & {
  params?: Record<string, string>;
};

function getBaseUrl(): string {
  return process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";
}

// Token refresh state — shared across all concurrent requests
let refreshPromise: Promise<boolean> | null = null;

async function attemptTokenRefresh(): Promise<boolean> {
  const refreshToken = getCookie("refresh_token");
  if (!refreshToken) return false;

  try {
    const baseUrl = getBaseUrl();
    const response = await fetch(new URL("/auth/refresh", baseUrl).toString(), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    if (!response.ok) return false;

    const data = await response.json();
    setCookie("auth_token", data.access_token, 1);
    setCookie("refresh_token", data.refresh_token, 7);
    return true;
  } catch {
    return false;
  }
}

async function refreshTokenWithMutex(): Promise<boolean> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = attemptTokenRefresh().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

function clearTokensAndRedirect(): void {
  removeCookie("auth_token");
  removeCookie("refresh_token");
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  options?: RequestOptions,
  isRetry = false,
): Promise<T> {
  const baseUrl = getBaseUrl();
  const url = new URL(path, baseUrl);

  if (options?.params) {
    for (const [key, value] of Object.entries(options.params)) {
      url.searchParams.set(key, value);
    }
  }

  const token = getCookie("auth_token");
  const headers = new Headers(options?.headers);

  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  if (
    body &&
    !(body instanceof URLSearchParams) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(url.toString(), {
    ...options,
    method,
    headers,
    credentials: "include",
    body:
      body instanceof URLSearchParams
        ? body
        : body
          ? JSON.stringify(body)
          : undefined,
  });

  if (!response.ok) {
    // Auto-refresh on 401 (skip for auth endpoints and retries)
    if (response.status === 401 && !isRetry && !path.startsWith("/auth/")) {
      const refreshed = await refreshTokenWithMutex();
      if (refreshed) {
        return request<T>(method, path, body, options, true);
      }
      clearTokensAndRedirect();
    }

    let errorData: {
      detail?: string;
      message?: string;
      errors?: Record<string, string[]>;
    } = {};
    try {
      errorData = await response.json();
    } catch {
      // Response body may not be JSON
    }
    throw new ApiError(
      response.status,
      errorData.detail || errorData.message || response.statusText,
      errorData.errors,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}

export const apiClient = {
  get<T>(path: string, options?: RequestOptions): Promise<T> {
    return request<T>("GET", path, undefined, options);
  },
  post<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return request<T>("POST", path, body, options);
  },
  put<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return request<T>("PUT", path, body, options);
  },
  patch<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
    return request<T>("PATCH", path, body, options);
  },
  delete<T>(path: string, options?: RequestOptions): Promise<T> {
    return request<T>("DELETE", path, undefined, options);
  },
};
