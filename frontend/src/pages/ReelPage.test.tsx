import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listReels, removeSavedVideo, saveVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import { authenticatedUser, publicVideo } from "../tests/videoFixtures";
import type { PublicVideo } from "../types/video";
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
    onVisibilityChange,
    onToggleSaved,
  }: {
    video: PublicVideo;
    isActive: boolean;
    shouldPreload: boolean;
    onVisibilityChange: (videoID: number, ratio: number) => void;
    onToggleSaved: (video: PublicVideo) => void;
  }) => (
    <article
      data-testid={`reel-${video.id}`}
      data-active={String(isActive)}
      data-preload={String(shouldPreload)}
    >
      <p>{video.title}</p>
      <p>{video.is_saved ? "saved" : "not-saved"}</p>
      <button type="button" onClick={() => onVisibilityChange(video.id, 0.8)}>
        visible-{video.id}
      </button>
      <button type="button" onClick={() => onToggleSaved(video)}>
        toggle-{video.id}
      </button>
    </article>
  ),
}));

const listReelsMock = vi.mocked(listReels);
const saveVideoMock = vi.mocked(saveVideo);
const removeSavedVideoMock = vi.mocked(removeSavedVideo);
const useAuthMock = vi.mocked(useAuth);

// Promise ChainとReact State更新を完了
async function flushAsync(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

// リール画面をRouter内で描画
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

  // ---追加---
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
  // ---追加---

  it("表示中の1件だけをActiveにして次の1件だけを先読みする", async () => {
    listReelsMock.mockResolvedValue({
      items: [
        publicVideo({ id: 10, title: "動画A" }),
        publicVideo({ id: 11, title: "動画B" }),
        publicVideo({ id: 12, title: "動画C" }),
      ],
      next_cursor: null,
      has_more: false,
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
});
