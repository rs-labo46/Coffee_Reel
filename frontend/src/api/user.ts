import type {
  AuthResponse,
  LoginInput,
  MeResponse,
  SignUpInput,
  SignUpResponse,
} from "../types/user";
import { apiRequest } from "./client";

export function signUp(input: SignUpInput): Promise<SignUpResponse> {
  return apiRequest<SignUpResponse>(
    "/signup",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    {
      auth: false,
      retryOnUnauthorized: false,
    },
  );
}

export function login(input: LoginInput): Promise<AuthResponse> {
  return apiRequest<AuthResponse>(
    "/login",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
    {
      auth: false,
      retryOnUnauthorized: false,
    },
  );
}

export function getMe(): Promise<MeResponse> {
  return apiRequest<MeResponse>(
    "/me",
    {
      method: "GET",
    },
    {
      retryOnUnauthorized: false,
    },
  );
}

export function logout(): Promise<void> {
  return apiRequest<void>(
    "/logout",
    {
      method: "POST",
    },
    {
      auth: false,
      csrf: true,
      retryOnUnauthorized: false,
    },
  );
}
