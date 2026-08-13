import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listReels, removeSavedVideo, saveVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import {
  authenticatedUser,
  guestUser,
  publicVideo,
} from "../tests/videoFixtures";
import type { PublicVideo, VideoLikeState } from "../types/video";
import ReelPage from "./ReelPage";

vi.mock("../api/video", () => ({
  listReels: vi.fn(),
  saveVideo: vi.fn(),
  removeSavedVideo: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("../components/ReelVideo", () => ({
  default: ({
    video,
    isActive,
    shouldPreload,
    isAuthenticated,
    onVisibilityChange,
    onToggleSaved,
    onLikeChange,
    onLikeNotFound,
  }: {
    video: PublicVideo;
    isActive: boolean;
    shouldPreload: boolean;
    isAuthenticated: boolean;
    onVisibilityChange: (videoID: number, ratio: number) => void;
    onToggleSaved: (video: PublicVideo) => void;
    onLikeChange: (state: VideoLikeState) => void;
    onLikeNotFound: (videoID: number) => void;
  }) => (
    <article
      data-testid={`reel-${video.id}`}
      data-active={String(isActive)}
      data-preload={String(shouldPreload)}
      data-authenticated={String(isAuthenticated)}
    >
      <p>{video.title}</p>
      <p>{video.is_saved ? "saved" : "not-saved"}</p>
      <p>{`Like ${video.like_count} ${video.is_liked ? "liked" : "not-liked"}`}</p>
      <button type="button" onClick={() => onVisibilityChange(video.id, 0.8)}>
        visible-{video.id}
      </button>
      <button type="button" onClick={() => onToggleSaved(video)}>
        toggle-{video.id}
      </button>
      <button
        type="button"
        onClick={() =>
          onLikeChange({
            video_id: video.id,
            like_count: video.like_count + 1,
            is_liked: true,
          })
        }
      >
        like-{video.id}
      </button>
      <button type="button" onClick={() => onLikeNotFound(video.id)}>
        like-not-found-{video.id}
      </button>
    </article>
  ),
}));

const listReelsMock = vi.mocked(listReels);
const saveVideoMock = vi.mocked(saveVideo);
const removeSavedVideoMock = vi.mocked(removeSavedVideo);
const useAuthMock = vi.mocked(useAuth);

async function flushAsync(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route path="/" element={<ReelPage />} />
        <Route path="/login" element={<p>LoginPage</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ReelPage", () => {
  beforeEach(() => {
    useAuthMock.mockReturnValue(authenticatedUser());
    listReelsMock.mockReset();
    saveVideoMock.mockReset();
    removeSavedVideoMock.mockReset();
  });

  it("認証復元中でも公開リール取得を開始して先に表示する", async () => {
    useAuthMock.mockReturnValue(guestUser({ isLoading: true }));
    listReelsMock.mockResolvedValue({
      items: [publicVideo({ id: 10, title: "先に表示する動画" })],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(listReelsMock).toHaveBeenCalledTimes(1);
    expect(screen.getByText("先に表示する動画")).toBeInTheDocument();
  });

  it("検索画面への導線を表示する", async () => {
    listReelsMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(screen.getByRole("link", { name: "検索" })).toHaveAttribute(
      "href",
      "/search",
    );
  });

  it("管理者には管理画面への導線をHeaderへ表示する", async () => {
    useAuthMock.mockReturnValue(
      authenticatedUser({
        user: {
          id: 2,
          name: "管理者",
          email: "admin@example.com",
          role: "admin",
          status: "active",
        },
      }),
    );
    listReelsMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(screen.getByRole("link", { name: "管理画面" })).toHaveAttribute(
      "href",
      "/admin/users",
    );
  });

  it("一般ユーザーには管理画面への導線を表示しない", async () => {
    listReelsMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(
      screen.queryByRole("link", { name: "管理画面" }),
    ).not.toBeInTheDocument();
  });

  it("HeaderからログアウトしてLogin画面へ移動する", async () => {
    const logout = vi.fn().mockResolvedValue(undefined);
    useAuthMock.mockReturnValue(authenticatedUser({ logout }));
    listReelsMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    fireEvent.click(screen.getByRole("button", { name: "ログアウト" }));
    await flushAsync();

    expect(logout).toHaveBeenCalledTimes(1);
    expect(screen.getByText("LoginPage")).toBeInTheDocument();
  });

  it("ログアウト処理中の二重送信を防ぐ", async () => {
    let resolveLogout: (() => void) | undefined;
    const logout = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveLogout = resolve;
        }),
    );
    useAuthMock.mockReturnValue(authenticatedUser({ logout }));
    listReelsMock.mockResolvedValue({
      items: [],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    const button = screen.getByRole("button", { name: "ログアウト" });
    fireEvent.click(button);
    fireEvent.click(button);

    expect(logout).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "ログアウト中" })).toBeDisabled();

    await act(async () => {
      resolveLogout?.();
      await Promise.resolve();
    });
  });

  it("表示中の1件だけをActiveにして次の1件だけを先読みする", async () => {
    listReelsMock.mockResolvedValue({
      items: [
        publicVideo({ id: 10, title: "動画A" }),
        publicVideo({ id: 11, title: "動画B" }),
        publicVideo({ id: 12, title: "動画C" }),
      ],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(screen.getByTestId("reel-10")).toHaveAttribute(
      "data-active",
      "true",
    );
    expect(screen.getByTestId("reel-11")).toHaveAttribute(
      "data-preload",
      "true",
    );
    expect(screen.getByTestId("reel-12")).toHaveAttribute(
      "data-preload",
      "false",
    );

    fireEvent.click(screen.getByRole("button", { name: "visible-11" }));

    expect(screen.getByTestId("reel-10")).toHaveAttribute(
      "data-active",
      "false",
    );
    expect(screen.getByTestId("reel-11")).toHaveAttribute(
      "data-active",
      "true",
    );
    expect(screen.getByTestId("reel-12")).toHaveAttribute(
      "data-preload",
      "true",
    );
  });

  it("公開動画を保存し、同じ画面から保存解除する", async () => {
    listReelsMock.mockResolvedValue({
      items: [publicVideo({ id: 10, is_saved: false })],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });
    saveVideoMock.mockResolvedValue(undefined);
    removeSavedVideoMock.mockResolvedValue(undefined);

    renderPage();
    await flushAsync();

    expect(screen.getByText("not-saved")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "toggle-10" }));
    await flushAsync();

    expect(saveVideoMock).toHaveBeenCalledWith(10);
    expect(screen.getByText("saved")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "toggle-10" }));
    await flushAsync();

    expect(removeSavedVideoMock).toHaveBeenCalledWith(10);
    expect(screen.getByText("not-saved")).toBeInTheDocument();
  });

  it("Like成功時にBackendが返した件数と状態を対象リールへ反映する", async () => {
    listReelsMock.mockResolvedValue({
      items: [publicVideo({ id: 10, like_count: 4, is_liked: false })],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    expect(screen.getByText("Like 4 not-liked")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "like-10" }));

    expect(screen.getByText("Like 5 liked")).toBeInTheDocument();
  });

  it("Like時に404相当を通知された動画を一覧から除外する", async () => {
    listReelsMock.mockResolvedValue({
      items: [
        publicVideo({ id: 10, title: "動画A" }),
        publicVideo({ id: 11, title: "動画B" }),
      ],
      next_cursor: null,
      has_more: false,
      result_type: "all",
    });

    renderPage();
    await flushAsync();

    fireEvent.click(screen.getByRole("button", { name: "like-not-found-10" }));

    expect(screen.queryByText("動画A")).not.toBeInTheDocument();
    expect(screen.getByText("動画B")).toBeInTheDocument();
  });
});
