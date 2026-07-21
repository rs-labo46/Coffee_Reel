import type {
  AuthResponse,
  LoginInput,
  SignUpInput,
  SignUpResponse,
} from "../types/user";
import { apiRequest } from "./client";

export function signUp(input: SignUpInput): Promise<SignUpResponse> {
  return apiRequest<SignUpResponse>("/signup", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });
}

export function login(input: LoginInput): Promise<AuthResponse> {
  return apiRequest<AuthResponse>("/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(input),
  });
}
