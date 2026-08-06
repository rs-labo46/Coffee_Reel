import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  getAdminVideo,
  hideAdminVideo,
  restoreAdminVideo,
} from "../api/admin_video";
import { ApiClientError } from "../api/client";

import AdminVideoDetailPage from "./AdminVideoDetailPage";
import type {
  AdminVideoDetailResponse,
  AdminVideoStateResponse,
} from "../types/admin_video";

vi.mock("../api/admin_video", () => ({
  getAdminVideo: vi.fn(),
  hideAdminVideo: vi.fn(),
  restoreAdminVideo: vi.fn(),
}));

const getAdminVideoMock = vi.mocked(getAdminVideo);
const hideAdminVideoMock = vi.mocked(hideAdminVideo);
const restoreAdminVideoMock = vi.mocked(restoreAdminVideo);

const publishedDetail: AdminVideoDetailResponse = {
  id: 10,
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
  playback_url: "https://storage.example/video.mp4",
  thumbnail_url: "https://storage.example/thumbnail.jpg",
  created_at: "2026-08-06T00:00:00Z",
  updated_at: "2026-08-06T01:00:00Z",
};

const hiddenDetail: AdminVideoDetailResponse = {
  ...publishedDetail,
  publish_status: "hidden",
};

const suspendedHiddenDetail: AdminVideoDetailResponse = {
  ...hiddenDetail,
  author: {
    ...hiddenDetail.author,
    status: "suspended",
  },
};

const hiddenState: AdminVideoStateResponse = {
  id: 10,
  processing_status: "ready",
  publish_status: "hidden",
  updated_at: "2026-08-06T02:00:00Z",
};

const publishedState: AdminVideoStateResponse = {
  ...hiddenState,
  publish_status: "published",
};

function renderPage(path = "/admin/videos/10") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          path="/admin/videos/:video_id"
          element={<AdminVideoDetailPage />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminVideoDetailPage", () => {
  beforeEach(() => {
    getAdminVideoMock.mockReset();
    hideAdminVideoMock.mockReset();
    restoreAdminVideoMock.mockReset();
  });

  it("不正なVideo IDならAPIを呼ばずErrorを表示する", () => {
    renderPage("/admin/videos/abc");

    expect(screen.getByRole("alert")).toHaveTextContent(
      "投稿IDが正しくありません",
    );
    expect(getAdminVideoMock).not.toHaveBeenCalled();
  });

  it("投稿者、投稿情報、状態、管理者確認用動画を表示する", async () => {
    getAdminVideoMock.mockResolvedValue(publishedDetail);

    renderPage();

    expect(screen.getByRole("status")).toHaveTextContent(
      "投稿詳細を取得しています",
    );
    expect(await screen.findByText("ハンドドリップの基本")).toBeInTheDocument();
    expect(screen.getByText("投稿者A")).toBeInTheDocument();
    expect(screen.getByText("抽出手順を説明します")).toBeInTheDocument();
    expect(screen.getByText("公開中")).toBeInTheDocument();
    expect(screen.getByLabelText("管理者確認用動画")).toBeInTheDocument();
    expect(screen.getByLabelText("非公開理由")).toBeInTheDocument();
    expect(screen.queryByLabelText("公開再開理由")).not.toBeInTheDocument();
    expect(getAdminVideoMock).toHaveBeenCalledWith(10);
  });

  it("管理者用URLがない状態では動画を表示しない", async () => {
    getAdminVideoMock.mockResolvedValue({
      ...publishedDetail,
      processing_status: "processing",
      publish_status: "private",
      playback_url: null,
      thumbnail_url: null,
    });

    renderPage();

    expect(
      await screen.findByText("現在の状態では管理者確認用動画を再生できません"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "投稿を非公開にする" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "投稿の公開を再開する" }),
    ).not.toBeInTheDocument();
  });

  it("空白だけの理由では非公開APIを呼ばない", async () => {
    const user = userEvent.setup();
    getAdminVideoMock.mockResolvedValue(publishedDetail);

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    await user.type(screen.getByLabelText("非公開理由"), "   ");
    await user.click(
      screen.getByRole("button", { name: "投稿を非公開にする" }),
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "理由は1文字以上500文字以内で入力してください",
    );
    expect(hideAdminVideoMock).not.toHaveBeenCalled();
  });

  it("理由をtrimして非公開にし、詳細を再取得する", async () => {
    const user = userEvent.setup();
    getAdminVideoMock
      .mockResolvedValueOnce(publishedDetail)
      .mockResolvedValueOnce(hiddenDetail);
    hideAdminVideoMock.mockResolvedValue(hiddenState);

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    await user.type(
      screen.getByLabelText("非公開理由"),
      "  利用規約違反を確認したため  ",
    );
    await user.click(
      screen.getByRole("button", { name: "投稿を非公開にする" }),
    );

    expect(hideAdminVideoMock).toHaveBeenCalledWith(10, {
      reason: "利用規約違反を確認したため",
    });
    expect(
      await screen.findByText("投稿を非公開にしました"),
    ).toBeInTheDocument();
    expect(screen.getByText("管理者により非公開")).toBeInTheDocument();
    expect(screen.getByLabelText("公開再開理由")).toHaveValue("");
    expect(getAdminVideoMock).toHaveBeenCalledTimes(2);
  });

  it("hiddenかつ投稿者activeなら公開再開できる", async () => {
    const user = userEvent.setup();
    getAdminVideoMock
      .mockResolvedValueOnce(hiddenDetail)
      .mockResolvedValueOnce(publishedDetail);
    restoreAdminVideoMock.mockResolvedValue(publishedState);

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    await user.type(
      screen.getByLabelText("公開再開理由"),
      "確認が完了したため",
    );
    await user.click(
      screen.getByRole("button", { name: "投稿の公開を再開する" }),
    );

    expect(restoreAdminVideoMock).toHaveBeenCalledWith(10, {
      reason: "確認が完了したため",
    });
    expect(
      await screen.findByText("投稿の公開を再開しました"),
    ).toBeInTheDocument();
    expect(screen.getByText("公開中")).toBeInTheDocument();
    expect(getAdminVideoMock).toHaveBeenCalledTimes(2);
  });

  it("hiddenでも投稿者suspendedなら公開再開を無効にする", async () => {
    const user = userEvent.setup();
    getAdminVideoMock.mockResolvedValue(suspendedHiddenDetail);

    renderPage();

    expect(
      await screen.findByText("投稿者が利用停止中のため、公開を再開できません"),
    ).toBeInTheDocument();

    const reason = screen.getByLabelText("公開再開理由");
    const button = screen.getByRole("button", {
      name: "投稿の公開を再開する",
    });

    expect(reason).toBeDisabled();
    expect(button).toBeDisabled();
    await user.click(button);
    expect(restoreAdminVideoMock).not.toHaveBeenCalled();
  });

  it("送信中は同じ非公開操作を二重送信しない", async () => {
    const user = userEvent.setup();
    let resolveHide: ((value: AdminVideoStateResponse) => void) | undefined;
    const hideRequest = new Promise<AdminVideoStateResponse>((resolve) => {
      resolveHide = resolve;
    });

    getAdminVideoMock
      .mockResolvedValueOnce(publishedDetail)
      .mockResolvedValueOnce(hiddenDetail);
    hideAdminVideoMock.mockReturnValue(hideRequest);

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    await user.type(screen.getByLabelText("非公開理由"), "規約違反");

    const button = screen.getByRole("button", {
      name: "投稿を非公開にする",
    });
    await user.click(button);

    expect(screen.getByRole("button", { name: "処理中" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "処理中" }));
    expect(hideAdminVideoMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveHide?.(hiddenState);
      await hideRequest;
    });

    expect(
      await screen.findByText("投稿を非公開にしました"),
    ).toBeInTheDocument();
  });

  it("409 Conflictなら競合を表示し、最新詳細を再取得する", async () => {
    const user = userEvent.setup();
    getAdminVideoMock
      .mockResolvedValueOnce(publishedDetail)
      .mockResolvedValueOnce(hiddenDetail);
    hideAdminVideoMock.mockRejectedValue(
      new ApiClientError(
        409,
        "video_state_conflict",
        "現在の状態では操作できません",
        "request-video-409",
      ),
    );

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    await user.type(screen.getByLabelText("非公開理由"), "重複操作の確認");
    await user.click(
      screen.getByRole("button", { name: "投稿を非公開にする" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "既に状態が変更されています",
    );
    expect(screen.getByText("管理者により非公開")).toBeInTheDocument();
    expect(getAdminVideoMock).toHaveBeenCalledTimes(2);
    expect(
      screen.queryByText("Request ID: request-video-409"),
    ).not.toBeInTheDocument();
  });

  it("詳細取得ErrorのMessageとRequest IDを表示する", async () => {
    getAdminVideoMock.mockRejectedValue(
      new ApiClientError(
        500,
        "internal_error",
        "投稿詳細を取得できませんでした",
        "request-video-detail",
      ),
    );

    renderPage();

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("投稿詳細を取得できませんでした");
    expect(alert).toHaveTextContent("Request ID: request-video-detail");
  });

  it("ユーザー管理と投稿管理への導線を表示する", async () => {
    getAdminVideoMock.mockResolvedValue(publishedDetail);

    renderPage();

    await screen.findByText("ハンドドリップの基本");
    expect(screen.getByRole("link", { name: "ユーザー管理" })).toHaveAttribute(
      "href",
      "/admin/users",
    );
    expect(
      screen.getByRole("link", { name: "投稿管理へ戻る" }),
    ).toHaveAttribute("href", "/admin/videos");
  });
});
