export interface LoginCredentials {
  account: string;
  password: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface UserResponse {
  id: number;
  account: string;
  is_disabled: boolean;
  role: number;
}
