import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  getAdminUser,
  resumeAdminUser,
  suspendAdminUser,
} from "../api/admin_user";
import { ApiClientError } from "../api/client";
import type {
  AdminUserDetailResponse,
  AdminUserStatusResponse,
} from "../types/admin_user";
import AdminUserDetailPage from "./AdminUserDetailPage";

vi.mock("../api/admin_user", () => ({
  getAdminUser: vi.fn(),
  resumeAdminUser: vi.fn(),
  suspendAdminUser: vi.fn(),
}));

const getAdminUserMock = vi.mocked(getAdminUser);
const suspendAdminUserMock = vi.mocked(suspendAdminUser);
const resumeAdminUserMock = vi.mocked(resumeAdminUser);

const activeDetail: AdminUserDetailResponse = {
  id: 10,
  name: "一般ユーザーA",
  email: "user-a@example.com",
  status: "active",
  created_at: "2026-07-25T00:00:00Z",
  videos: [
    {
      id: 101,
      title: "ハンドドリップの基本",
      processing_status: "ready",
      publish_status: "published",
      created_at: "2026-07-25T01:00:00Z",
    },
  ],
};

const suspendedDetail: AdminUserDetailResponse = {
  ...activeDetail,
  status: "suspended",
  videos: [
    {
      ...activeDetail.videos[0],
      publish_status: "hidden",
    },
  ],
};

const suspendedStatus: AdminUserStatusResponse = {
  id: 10,
  status: "suspended",
  updated_at: "2026-07-26T00:00:00Z",
};

const activeStatus: AdminUserStatusResponse = {
  id: 10,
  status: "active",
  updated_at: "2026-07-26T01:00:00Z",
};

function renderPage(path = "/admin/users/10") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/admin/users/:user_id" element={<AdminUserDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminUserDetailPage", () => {
  beforeEach(() => {
    getAdminUserMock.mockReset();
    suspendAdminUserMock.mockReset();
    resumeAdminUserMock.mockReset();
  });

  it("不正なUser IDならAPIを呼ばずErrorを表示する", () => {
    renderPage("/admin/users/abc");

    expect(screen.getByRole("alert")).toHaveTextContent(
      "ユーザーIDが正しくありません",
    );
    expect(getAdminUserMock).not.toHaveBeenCalled();
  });

  it("ユーザー情報と投稿動画を表示する", async () => {
    getAdminUserMock.mockResolvedValue(activeDetail);

    renderPage();

    expect(screen.getByRole("status")).toHaveTextContent(
      "ユーザー詳細を取得しています",
    );
    expect(await screen.findByText("一般ユーザーA")).toBeInTheDocument();
    expect(screen.getByText("user-a@example.com")).toBeInTheDocument();
    expect(screen.getByText("利用中")).toBeInTheDocument();
    expect(screen.getByText("ハンドドリップの基本")).toBeInTheDocument();
    expect(screen.getByText("published")).toBeInTheDocument();
    expect(
      screen.getByRole("link", {
        name: "ハンドドリップの基本の投稿詳細を開く",
      }),
    ).toHaveAttribute("href", "/admin/videos/101");
    expect(screen.getByRole("link", { name: "投稿管理" })).toHaveAttribute(
      "href",
      "/admin/videos",
    );
    expect(getAdminUserMock).toHaveBeenCalledWith(10);
  });

  it("空白だけの理由では利用停止APIを呼ばない", async () => {
    const user = userEvent.setup();
    getAdminUserMock.mockResolvedValue(activeDetail);

    renderPage();

    await screen.findByText("一般ユーザーA");
    await user.type(screen.getByLabelText("停止理由"), "   ");
    await user.click(
      screen.getByRole("button", { name: "ユーザーを利用停止にする" }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "理由は1文字以上500文字以内で入力してください",
    );
    expect(suspendAdminUserMock).not.toHaveBeenCalled();
  });

  it("理由をtrimして利用停止し、詳細を再取得する", async () => {
    const user = userEvent.setup();
    getAdminUserMock
      .mockResolvedValueOnce(activeDetail)
      .mockResolvedValueOnce(suspendedDetail);
    suspendAdminUserMock.mockResolvedValue(suspendedStatus);

    renderPage();

    await screen.findByText("一般ユーザーA");
    await user.type(
      screen.getByLabelText("停止理由"),
      "  利用規約違反を確認したため  ",
    );
    await user.click(
      screen.getByRole("button", { name: "ユーザーを利用停止にする" }),
    );

    expect(suspendAdminUserMock).toHaveBeenCalledWith(10, {
      reason: "利用規約違反を確認したため",
    });
    expect(
      await screen.findByText("ユーザーを利用停止にしました"),
    ).toBeInTheDocument();
    expect(screen.getByText("利用停止中")).toBeInTheDocument();
    expect(screen.getByText("hidden")).toBeInTheDocument();
    expect(getAdminUserMock).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText("再開理由")).toHaveValue("");
  });

  it("停止中ユーザーを再開し、ResponseのStatusを反映する", async () => {
    const user = userEvent.setup();
    getAdminUserMock.mockResolvedValue(suspendedDetail);
    resumeAdminUserMock.mockResolvedValue(activeStatus);

    renderPage();

    await screen.findByText("一般ユーザーA");
    await user.type(screen.getByLabelText("再開理由"), "確認が完了したため");
    await user.click(
      screen.getByRole("button", { name: "ユーザーの利用を再開する" }),
    );

    expect(resumeAdminUserMock).toHaveBeenCalledWith(10, {
      reason: "確認が完了したため",
    });
    expect(
      await screen.findByText("ユーザーの利用を再開しました"),
    ).toBeInTheDocument();
    expect(screen.getByText("利用中")).toBeInTheDocument();
    expect(screen.getByText("hidden")).toBeInTheDocument();
    expect(getAdminUserMock).toHaveBeenCalledTimes(1);
  });

  it("409 Conflictなら競合を表示し、最新詳細を再取得する", async () => {
    const user = userEvent.setup();
    getAdminUserMock
      .mockResolvedValueOnce(activeDetail)
      .mockResolvedValueOnce(suspendedDetail);
    suspendAdminUserMock.mockRejectedValue(
      new ApiClientError(
        409,
        "admin_user_state_conflict",
        "現在の状態では操作できません",
        "request-409",
      ),
    );

    renderPage();

    await screen.findByText("一般ユーザーA");
    await user.type(screen.getByLabelText("停止理由"), "重複操作の確認");
    await user.click(
      screen.getByRole("button", { name: "ユーザーを利用停止にする" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "既に状態が変更されています",
    );
    expect(screen.getByText("利用停止中")).toBeInTheDocument();
    expect(getAdminUserMock).toHaveBeenCalledTimes(2);
    expect(
      screen.queryByText("Request ID: request-409"),
    ).not.toBeInTheDocument();
  });

  it("詳細取得ErrorのMessageとRequest IDを表示する", async () => {
    getAdminUserMock.mockRejectedValue(
      new ApiClientError(
        500,
        "internal_error",
        "ユーザー詳細を取得できませんでした",
        "request-500",
      ),
    );

    renderPage();

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("ユーザー詳細を取得できませんでした");
    expect(alert).toHaveTextContent("Request ID: request-500");
  });
});
