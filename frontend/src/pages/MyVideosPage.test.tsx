import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import {
  deleteVideo,
  getMyVideo,
  listMyVideos,
  republishVideo,
  setVideoPrivate,
} from "../api/video";
import { useAuth } from "../auth/useAuth";
import {
  authenticatedUser,
  ownedVideo,
  ownedVideoDetail,
} from "../tests/videoFixtures";
import type { OwnedVideoDetail, VideoProcessingStatus } from "../types/video";
import MyVideosPage from "./MyVideosPage";

vi.mock("../api/video", () => ({
  listMyVideos: vi.fn(),
  getMyVideo: vi.fn(),
  setVideoPrivate: vi.fn(),
  republishVideo: vi.fn(),
  deleteVideo: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("../components/VideoCard", () => ({
  default: ({
    video,
    action,
  }: {
    video: {
      id: number;
      title: string;
      processing_status: string;
      publish_status: string;
    };
    action?: React.ReactNode;
  }) => (
    <article aria-label={`video-${video.id}`}>
      <p>{video.title}</p>
      <p>{video.processing_status}</p>
      <p>{video.publish_status}</p>
      {action}
    </article>
  ),
}));

const listMyVideosMock = vi.mocked(listMyVideos);
const getMyVideoMock = vi.mocked(getMyVideo);
const setVideoPrivateMock = vi.mocked(setVideoPrivate);
const republishVideoMock = vi.mocked(republishVideo);
const deleteVideoMock = vi.mocked(deleteVideo);
const useAuthMock = vi.mocked(useAuth);

// Promise ChainとReact State更新を完了
async function flushAsync(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

// Upload完了直後のRouter State付きで投稿一覧を描画
function renderPollingPage() {
  return render(
    <MemoryRouter
      initialEntries={[
        {
          pathname: "/me/videos",
          state: {
            uploadCompleted: true,
            videoID: 10,
          },
        },
      ]}
    >
      <MyVideosPage />
    </MemoryRouter>,
  );
}

// 通常の自分の投稿一覧を描画
function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/me/videos"]}>
      <MyVideosPage />
    </MemoryRouter>,
  );
}

describe("MyVideosPage", () => {
  beforeEach(() => {
    useAuthMock.mockReturnValue(authenticatedUser());
    listMyVideosMock.mockReset();
    getMyVideoMock.mockReset();
    setVideoPrivateMock.mockReset();
    republishVideoMock.mockReset();
    deleteVideoMock.mockReset();

    listMyVideosMock.mockResolvedValue({
      items: [ownedVideo()],
      next_cursor: null,
      has_more: false,
    });
  });

  it.each<{
    status: Extract<VideoProcessingStatus, "ready" | "failed" | "expired">;
    failureCode: OwnedVideoDetail["failure_code"];
  }>([
    { status: "ready", failureCode: null },
    { status: "failed", failureCode: "video_corrupt" },
    { status: "expired", failureCode: null },
  ])("$statusでPollingを停止する", async ({ status, failureCode }) => {
    const setTimeoutSpy = vi
      .spyOn(window, "setTimeout")
      .mockImplementation(() => 1);

    getMyVideoMock.mockResolvedValue(
      ownedVideoDetail({
        processing_status: status,
        publish_status: status === "ready" ? "published" : "private",
        failure_code: failureCode,
      }),
    );

    renderPollingPage();
    await flushAsync();

    expect(getMyVideoMock).toHaveBeenCalledTimes(1);
    expect(getMyVideoMock).toHaveBeenCalledWith(10);
    expect(setTimeoutSpy).not.toHaveBeenCalled();
  });

  it("詳細APIの404で対象Videoを除外しPollingを停止する", async () => {
    const setTimeoutSpy = vi
      .spyOn(window, "setTimeout")
      .mockImplementation(() => 1);

    getMyVideoMock.mockRejectedValue(
      new ApiClientError(404, "video_not_found", "動画が見つかりません"),
    );

    renderPollingPage();
    await flushAsync();

    expect(screen.queryByText("ハンドドリップの蒸らし方")).not.toBeInTheDocument();
    expect(getMyVideoMock).toHaveBeenCalledTimes(1);
    expect(setTimeoutSpy).not.toHaveBeenCalled();
  });

  it("削除成功時に待機中Pollingを無効化する", async () => {
    let scheduledPolling: (() => void) | null = null;

    vi.spyOn(window, "setTimeout").mockImplementation((handler) => {
      scheduledPolling = handler as () => void;
      return 1;
    });
    vi.spyOn(window, "confirm").mockReturnValue(true);

    getMyVideoMock.mockResolvedValue(
      ownedVideoDetail({
        processing_status: "processing",
      }),
    );
    deleteVideoMock.mockResolvedValue(undefined);

    renderPollingPage();
    await flushAsync();

    expect(getMyVideoMock).toHaveBeenCalledTimes(1);
    expect(scheduledPolling).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "削除" }));
    await flushAsync();

    expect(deleteVideoMock).toHaveBeenCalledWith(10);
    expect(screen.queryByText("ハンドドリップの蒸らし方")).not.toBeInTheDocument();

    await act(async () => {
      scheduledPolling?.();
      await Promise.resolve();
    });

    expect(getMyVideoMock).toHaveBeenCalledTimes(1);
  });

  it("hidden動画へ再公開Buttonを表示しない", async () => {
    listMyVideosMock.mockResolvedValue({
      items: [
        ownedVideo({
          processing_status: "ready",
          publish_status: "hidden",
        }),
      ],
      next_cursor: null,
      has_more: false,
    });

    renderPage();
    await flushAsync();

    expect(screen.getByText("hidden")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "再公開" })).not.toBeInTheDocument();
  });

  it("failure_codeを安全な固定Messageへ変換しWorker内部Messageを表示しない", async () => {
    const failedDetail = {
      ...ownedVideoDetail({
        processing_status: "failed",
        failure_code: "processing_failed",
      }),
      worker_error_message: "ffmpeg command failed with secret path",
    } as OwnedVideoDetail;

    getMyVideoMock.mockResolvedValue(failedDetail);

    renderPollingPage();
    await flushAsync();

    expect(
      screen.getByText(
        "動画処理に失敗しました。時間を置いて再投稿してください",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("ffmpeg command failed with secret path"),
    ).not.toBeInTheDocument();
  });
});
