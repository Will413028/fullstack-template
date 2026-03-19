// Common type definitions

export interface User {
  id: string;
  email: string;
  name: string;
  avatar?: string;
  role: "admin" | "user";
  createdAt: string;
  updatedAt: string;
}

export interface ApiResponse<T> {
  data: T;
  message?: string;
  success: boolean;
}

export interface ApiError {
  status: number;
  message: string;
  errors?: Record<string, string[]>;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    page: number;
    pageSize: number;
    total: number;
    totalPages: number;
  };
}

export interface NavLink {
  href: string;
  label: string;
  icon?: React.ComponentType<{ size?: number }>;
}

export interface SocialLink {
  platform: string;
  url: string;
  icon: React.ComponentType<{ size?: number }>;
}
