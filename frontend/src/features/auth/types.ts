export interface LoginCredentials {
  account: string;
  password: string;
}

export interface UserResponse {
  id: number;
  account: string;
  is_disabled: boolean;
  role: number;
}
