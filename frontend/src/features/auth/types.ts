export interface LoginCredentials {
  email: string;
  password: string;
}

export interface AuthResponse {
  user: {
    id: string;
    email: string;
    name: string;
  };
  token: string;
}

export interface AuthState {
  user: AuthResponse["user"] | null;
  isAuthenticated: boolean;
}
