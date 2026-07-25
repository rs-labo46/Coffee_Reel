import type {
  AdminUserDetailResponse,
  AdminUserListResponse,
  AdminUserReasonInput,
  AdminUserStatusResponse,
} from "../types/admin_user";
import { apiRequest } from "./client";

const adminUsersPath = "/admin/users";

export function listAdminUsers(
  cursor: string | null = null,
  limit = 20,
): Promise<AdminUserListResponse> {
  const query = new URLSearchParams({ limit: String(limit) });

  if (cursor !== null) {
    query.set("cursor", cursor);
  }

  return apiRequest<AdminUserListResponse>(
    `${adminUsersPath}?${query.toString()}`,
    {
      method: "GET",
    },
  );
}

export function getAdminUser(userID: number): Promise<AdminUserDetailResponse> {
  return apiRequest<AdminUserDetailResponse>(`${adminUsersPath}/${userID}`, {
    method: "GET",
  });
}

export function suspendAdminUser(
  userID: number,
  input: AdminUserReasonInput,
): Promise<AdminUserStatusResponse> {
  return changeAdminUserStatus(userID, "suspend", input);
}

export function resumeAdminUser(
  userID: number,
  input: AdminUserReasonInput,
): Promise<AdminUserStatusResponse> {
  return changeAdminUserStatus(userID, "resume", input);
}

function changeAdminUserStatus(
  userID: number,
  action: "suspend" | "resume",
  input: AdminUserReasonInput,
): Promise<AdminUserStatusResponse> {
  return apiRequest<AdminUserStatusResponse>(
    `${adminUsersPath}/${userID}/${action}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}
