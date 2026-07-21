export type UserRole = "user" | "admin";
export type UserStatus = "active" | "suspended";

export type User = {
  id: number;
  name: string;
  email: string;
  role: UserRole;
  status: UserStatus;
};

export type SignUpInput = {
  name: string;
  email: string;
  password: string;
};

export type SignUpResponse = {
  data: User & { created_at: string };
};

export type ApiErrorResponse = {
  status: number;
  code: string;
  message: string;
  request_id: string;
};
