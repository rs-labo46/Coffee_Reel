import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listSavedVideos, removeSavedVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import { authenticatedUser, publicVideo } from "../tests/videoFixtures";
import type { PublicVideo } from "../types/video";
import SavedVideosPage from "./SavedVideosPage";

vi.mock("../api/video", () => ({
  listSavedVideos: vi.fn(),
  removeSavedVideo: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("../components/VideoCard", () => ({
  default: ({
    video,
    action,
  }: {
    video: PublicVideo;
    action?: React.ReactNode;
  }) => (
    <article aria-label={`saved-video-${video.id}`}>
      <p>{video.title}</p>
      {action}
    </article>
  ),
}));

const listSavedVideosMock = vi.mocked(listSavedVideos);
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

// 保存一覧をRouter内で描画
function renderPage() {
  return render(
    <MemoryRouter>
      <SavedVideosPage />
    </MemoryRouter>,
  );
}

describe("SavedVideosPage", () => {
  beforeEach(() => {
    useAuthMock.mockReturnValue(authenticatedUser());
    listSavedVideosMock.mockReset();
    removeSavedVideoMock.mockReset();
  });

  it("次Cursorの保存一覧を重複なしで末尾へ追加する", async () => {
    listSavedVideosMock
      .mockResolvedValueOnce({
        items: [publicVideo({ id: 10, title: "動画A", is_saved: true })],
        next_cursor: "next-cursor",
        has_more: true,
      })
      .mockResolvedValueOnce({
        items: [
          publicVideo({ id: 10, title: "動画A", is_saved: true }),
          publicVideo({ id: 11, title: "動画B", is_saved: true }),
        ],
        next_cursor: null,
        has_more: false,
      });

    renderPage();
    await flushAsync();

    expect(screen.getByText("動画A")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "さらに読み込む" }));
    await flushAsync();

    expect(listSavedVideosMock).toHaveBeenNthCalledWith(1);
    expect(listSavedVideosMock).toHaveBeenNthCalledWith(2, {
      cursor: "next-cursor",
    });
    expect(screen.getAllByText("動画A")).toHaveLength(1);
    expect(screen.getByText("動画B")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "さらに読み込む" }),
    ).not.toBeInTheDocument();
  });

  it("保存解除成功後に対象動画を一覧から除外する", async () => {
    listSavedVideosMock.mockResolvedValue({
      items: [publicVideo({ id: 12, title: "解除対象", is_saved: true })],
      next_cursor: null,
      has_more: false,
    });
    removeSavedVideoMock.mockResolvedValue(undefined);

    renderPage();
    await flushAsync();

    const removeButton = screen.getByRole("button", { name: "保存を解除" });
    expect(removeButton).toHaveTextContent("");
    expect(removeButton.querySelector("svg")).not.toBeNull();

    fireEvent.click(removeButton);
    await flushAsync();

    expect(removeSavedVideoMock).toHaveBeenCalledWith(12);
    expect(screen.queryByText("解除対象")).not.toBeInTheDocument();
    expect(
      screen.getByText("保存した動画はまだありません"),
    ).toBeInTheDocument();
  });
});
