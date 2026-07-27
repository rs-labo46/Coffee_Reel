import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listAdminUsers } from "../api/admin_user";
import { ApiClientError } from "../api/client";
import type { AdminUserListResponse } from "../types/admin_user";
import AdminUsersPage from "./AdminUsersPage";

vi.mock("../api/admin_user", () => ({
  listAdminUsers: vi.fn(),
}));

const listAdminUsersMock = vi.mocked(listAdminUsers);

const firstPage: AdminUserListResponse = {
  items: [
    {
      id: 10,
      name: "一般ユーザーA",
      email: "user-a@example.com",
      status: "active",
      created_at: "2026-07-25T00:00:00Z",
    },
  ],
  next_cursor: "next-cursor",
  has_more: true,
};

const lastPage: AdminUserListResponse = {
  items: [
    {
      id: 9,
      name: "一般ユーザーB",
      email: "user-b@example.com",
      status: "suspended",
      created_at: "2026-07-24T00:00:00Z",
    },
  ],
  next_cursor: null,
  has_more: false,
};

function renderPage() {
  return render(
    <MemoryRouter>
      <AdminUsersPage />
    </MemoryRouter>,
  );
}

describe("AdminUsersPage", () => {
  beforeEach(() => {
    listAdminUsersMock.mockReset();
  });

  it("初回表示中はLoadingを表示し、一覧APIへlimit 20を渡す", async () => {
    let resolveRequest: ((value: AdminUserListResponse) => void) | undefined;

    const request = new Promise<AdminUserListResponse>((resolve) => {
      resolveRequest = resolve;
    });

    listAdminUsersMock.mockReturnValue(request);

    renderPage();

    expect(screen.getByRole("status")).toHaveTextContent(
      "ユーザー一覧を取得しています",
    );
    expect(listAdminUsersMock).toHaveBeenCalledWith(null, 20);

    await act(async () => {
      resolveRequest?.({
        items: [],
        next_cursor: null,
        has_more: false,
      });
      await request;
    });
  });

  it("取得したユーザー情報と詳細リンクを表示する", async () => {
    listAdminUsersMock.mockResolvedValue({
      ...firstPage,
      has_more: false,
      next_cursor: null,
    });

    renderPage();

    expect(await screen.findByText("一般ユーザーA")).toBeInTheDocument();
    expect(screen.getByText("user-a@example.com")).toBeInTheDocument();
    expect(screen.getByText("利用中")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "詳細を確認" })).toHaveAttribute(
      "href",
      "/admin/users/10",
    );
  });

  it("ユーザーが0件なら空状態を表示する", async () => {
    listAdminUsersMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
    });

    renderPage();

    expect(
      await screen.findByText("一般ユーザーは登録されていません"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "さらに読み込む" }),
    ).not.toBeInTheDocument();
  });

  it("次Cursorで追加取得し、既存一覧の末尾へ追加する", async () => {
    const user = userEvent.setup();

    listAdminUsersMock
      .mockResolvedValueOnce(firstPage)
      .mockResolvedValueOnce(lastPage);

    renderPage();

    expect(await screen.findByText("一般ユーザーA")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "さらに読み込む" }));

    expect(await screen.findByText("一般ユーザーB")).toBeInTheDocument();
    expect(screen.getByText("一般ユーザーA")).toBeInTheDocument();
    expect(screen.getByText("利用停止中")).toBeInTheDocument();

    expect(listAdminUsersMock).toHaveBeenNthCalledWith(1, null, 20);
    expect(listAdminUsersMock).toHaveBeenNthCalledWith(2, "next-cursor", 20);

    expect(
      screen.queryByRole("button", { name: "さらに読み込む" }),
    ).not.toBeInTheDocument();
  });

  it("API ErrorのMessageとRequest IDを表示する", async () => {
    listAdminUsersMock.mockRejectedValue(
      new ApiClientError(
        500,
        "internal_error",
        "ユーザー一覧を取得できませんでした",
        "request-123",
      ),
    );

    renderPage();

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("ユーザー一覧を取得できませんでした");
    expect(alert).toHaveTextContent("Request ID: request-123");
  });
});
