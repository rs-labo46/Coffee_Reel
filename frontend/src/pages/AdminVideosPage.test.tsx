import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listAdminVideos } from "../api/admin_video";
import { ApiClientError } from "../api/client";

import AdminVideosPage from "./AdminVideosPage";
import type { AdminVideoListResponse } from "../types/admin_video";

vi.mock("../api/admin_video", () => ({
  listAdminVideos: vi.fn(),
}));

const listAdminVideosMock = vi.mocked(listAdminVideos);

const firstPage: AdminVideoListResponse = {
  items: [
    {
      id: 20,
      author: {
        id: 2,
        name: "投稿者A",
        status: "active",
      },
      title: "ハンドドリップの基本",
      description: "抽出手順を説明します",
      category: "brewing",
      processing_status: "ready",
      publish_status: "published",
      created_at: "2026-08-06T02:00:00Z",
      updated_at: "2026-08-06T02:00:00Z",
    },
  ],
  next_cursor: "next-cursor",
  has_more: true,
};

const lastPage: AdminVideoListResponse = {
  items: [
    {
      id: 19,
      author: {
        id: 3,
        name: "投稿者B",
        status: "suspended",
      },
      title: "焙煎の記録",
      description: "浅煎りの変化を記録しました",
      category: "roasting",
      processing_status: "failed",
      publish_status: "private",
      created_at: "2026-08-06T01:00:00Z",
      updated_at: "2026-08-06T01:00:00Z",
    },
  ],
  next_cursor: null,
  has_more: false,
};

function renderPage() {
  return render(
    <MemoryRouter>
      <AdminVideosPage />
    </MemoryRouter>,
  );
}

describe("AdminVideosPage", () => {
  beforeEach(() => {
    listAdminVideosMock.mockReset();
  });

  it("初回表示中はLoadingを表示し、一覧APIへlimit 20を渡す", async () => {
    let resolveRequest: ((value: AdminVideoListResponse) => void) | undefined;

    const request = new Promise<AdminVideoListResponse>((resolve) => {
      resolveRequest = resolve;
    });

    listAdminVideosMock.mockReturnValue(request);

    renderPage();

    expect(screen.getByRole("status")).toHaveTextContent(
      "投稿一覧を取得しています",
    );
    expect(listAdminVideosMock).toHaveBeenCalledWith(null, 20);

    await act(async () => {
      resolveRequest?.({
        items: [],
        next_cursor: null,
        has_more: false,
      });
      await request;
    });
  });

  it("全状態の投稿情報と詳細リンクを表示する", async () => {
    listAdminVideosMock.mockResolvedValue({
      items: [...firstPage.items, ...lastPage.items],
      next_cursor: null,
      has_more: false,
    });

    renderPage();

    expect(await screen.findByText("ハンドドリップの基本")).toBeInTheDocument();
    expect(screen.getByText("抽出手順を説明します")).toBeInTheDocument();
    expect(screen.getByText("投稿者: 投稿者A")).toBeInTheDocument();
    expect(screen.getByText("公開中")).toBeInTheDocument();
    expect(screen.getByText("動画処理失敗")).toBeInTheDocument();
    expect(screen.getByText("投稿者: 利用停止中")).toBeInTheDocument();

    const detailLinks = screen.getAllByRole("link", { name: "詳細を確認" });
    expect(detailLinks[0]).toHaveAttribute("href", "/admin/videos/20");
    expect(detailLinks[1]).toHaveAttribute("href", "/admin/videos/19");
  });

  it("投稿が0件なら空状態を表示する", async () => {
    listAdminVideosMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
    });

    renderPage();

    expect(
      await screen.findByText("管理対象の投稿はありません"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "さらに読み込む" }),
    ).not.toBeInTheDocument();
  });

  it("次Cursorで追加取得し、既存一覧の末尾へ追加する", async () => {
    const user = userEvent.setup();

    listAdminVideosMock
      .mockResolvedValueOnce(firstPage)
      .mockResolvedValueOnce(lastPage);

    renderPage();

    expect(await screen.findByText("ハンドドリップの基本")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "さらに読み込む" }));

    expect(await screen.findByText("焙煎の記録")).toBeInTheDocument();
    expect(screen.getByText("ハンドドリップの基本")).toBeInTheDocument();
    expect(listAdminVideosMock).toHaveBeenNthCalledWith(1, null, 20);
    expect(listAdminVideosMock).toHaveBeenNthCalledWith(2, "next-cursor", 20);
    expect(
      screen.queryByRole("button", { name: "さらに読み込む" }),
    ).not.toBeInTheDocument();
  });

  it("API ErrorのMessageとRequest IDを表示する", async () => {
    listAdminVideosMock.mockRejectedValue(
      new ApiClientError(
        500,
        "internal_error",
        "投稿一覧を取得できませんでした",
        "request-video-list",
      ),
    );

    renderPage();

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("投稿一覧を取得できませんでした");
    expect(alert).toHaveTextContent("Request ID: request-video-list");
  });

  it("ユーザー管理と投稿管理の相互リンクを表示する", async () => {
    listAdminVideosMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
    });

    renderPage();

    await screen.findByText("管理対象の投稿はありません");
    expect(screen.getByRole("link", { name: "ユーザー管理" })).toHaveAttribute(
      "href",
      "/admin/users",
    );
    expect(screen.getByRole("link", { name: "投稿管理" })).toHaveAttribute(
      "href",
      "/admin/videos",
    );
  });
});
