import { beforeEach, describe, expect, it, vi } from "vitest";

import type {
  AdminUserDetailResponse,
  AdminUserListResponse,
  AdminUserStatusResponse,
} from "../types/admin_user";
import {
  getAdminUser,
  listAdminUsers,
  resumeAdminUser,
  suspendAdminUser,
} from "./admin_user";
import { apiRequest } from "./client";

vi.mock("./client", () => ({
  apiRequest: vi.fn(),
}));

const apiRequestMock = vi.mocked(apiRequest);

const listResponse: AdminUserListResponse = {
  items: [],
  next_cursor: null,
  has_more: false,
};

const detailResponse: AdminUserDetailResponse = {
  id: 10,
  name: "一般ユーザー",
  email: "user@example.com",
  status: "active",
  created_at: "2026-07-25T00:00:00Z",
  videos: [],
};

const statusResponse: AdminUserStatusResponse = {
  id: 10,
  status: "suspended",
  updated_at: "2026-07-25T01:00:00Z",
};

describe("管理者ユーザーAPI", () => {
  beforeEach(() => {
    apiRequestMock.mockReset();
  });

  it("Cursorなしでユーザー一覧APIを呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(listResponse);

    await expect(listAdminUsers()).resolves.toEqual(listResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/users?limit=20", {
      method: "GET",
    });
  });

  it("CursorをURLエンコードして次のユーザー一覧を取得する", async () => {
    apiRequestMock.mockResolvedValue(listResponse);

    await listAdminUsers("cursor+/=value", 50);

    expect(apiRequestMock).toHaveBeenCalledWith(
      "/admin/users?limit=50&cursor=cursor%2B%2F%3Dvalue",
      {
        method: "GET",
      },
    );
  });

  it("User IDをPathへ設定してユーザー詳細APIを呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(detailResponse);

    await expect(getAdminUser(10)).resolves.toEqual(detailResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/users/10", {
      method: "GET",
    });
  });

  it("停止理由をJSONで送信してユーザーを利用停止する", async () => {
    apiRequestMock.mockResolvedValue(statusResponse);

    await expect(
      suspendAdminUser(10, { reason: "利用規約違反を確認したため" }),
    ).resolves.toEqual(statusResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/users/10/suspend", {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        reason: "利用規約違反を確認したため",
      }),
    });
  });

  it("再開理由をJSONで送信してユーザーの利用を再開する", async () => {
    apiRequestMock.mockResolvedValue({
      ...statusResponse,
      status: "active",
    });

    await resumeAdminUser(10, {
      reason: "確認が完了したため",
    });

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/users/10/resume", {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        reason: "確認が完了したため",
      }),
    });
  });
});
