import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
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
      <button
        type="button"
        onClick={() => onVisibilityChange(video.id, 0.8)}
      >
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
    <MemoryRouter>
      <ReelPage />
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
