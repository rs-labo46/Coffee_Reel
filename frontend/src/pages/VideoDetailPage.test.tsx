import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { getVideoDetail, removeSavedVideo, saveVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import { authenticatedUser, publicVideo } from "../tests/videoFixtures";
import VideoDetailPage from "./VideoDetailPage";

vi.mock("../api/video", () => ({
  getVideoDetail: vi.fn(),
  saveVideo: vi.fn(),
  removeSavedVideo: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const getVideoDetailMock = vi.mocked(getVideoDetail);
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

// 指定Video IDの詳細Routeを描画
function renderDetail(videoID: string) {
  return render(
    <MemoryRouter initialEntries={[`/videos/${videoID}`]}>
      <Routes>
        <Route path="/videos/:video_id" element={<VideoDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("VideoDetailPage", () => {
  beforeEach(() => {
    useAuthMock.mockReturnValue(authenticatedUser());
    getVideoDetailMock.mockReset();
    saveVideoMock.mockReset();
    removeSavedVideoMock.mockReset();
  });

  it("公開動画の再生情報と投稿者情報を表示する", async () => {
    getVideoDetailMock.mockResolvedValue(publicVideo({ id: 20 }));

    renderDetail("20");
    await flushAsync();

    expect(screen.getByRole("heading", { name: "ハンドドリップの蒸らし方" })).toBeInTheDocument();
    expect(screen.getByText("コーヒー太郎")).toBeInTheDocument();
    expect(screen.getByText("抽出")).toBeInTheDocument();
    expect(screen.getByText("30秒蒸らしてからゆっくり注ぎます")).toBeInTheDocument();
    expect(screen.getByText("保存一覧")).toHaveAttribute(
      "href",
      "/me/saved-videos",
    );
  });

  it("TitleとDescription内のHTML文字列を実行しない", async () => {
    const title = "<script>window.hacked=true</script>";
    const description = '<img src=x onerror="window.hacked=true">';
    getVideoDetailMock.mockResolvedValue(
      publicVideo({
        id: 21,
        title,
        description,
      }),
    );

    const { container } = renderDetail("21");
    await flushAsync();

    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText(description)).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector('img[src="x"]')).toBeNull();
  });

  it("詳細画面で保存と保存解除を切り替える", async () => {
    getVideoDetailMock.mockResolvedValue(
      publicVideo({
        id: 22,
        is_saved: false,
      }),
    );
    saveVideoMock.mockResolvedValue(undefined);
    removeSavedVideoMock.mockResolvedValue(undefined);

    renderDetail("22");
    await flushAsync();

    fireEvent.click(screen.getByRole("button", { name: "動画を保存" }));
    await flushAsync();

    expect(saveVideoMock).toHaveBeenCalledWith(22);
    expect(
      screen.getByRole("button", { name: "保存を解除" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "保存を解除" }));
    await flushAsync();

    expect(removeSavedVideoMock).toHaveBeenCalledWith(22);
    expect(
      screen.getByRole("button", { name: "動画を保存" }),
    ).toBeInTheDocument();
  });

  it("公開対象外の404を動画が見つからない表示へ変換する", async () => {
    getVideoDetailMock.mockRejectedValue(
      new ApiClientError(
        404,
        "video_not_found",
        "動画が見つかりません",
        "request-404",
      ),
    );

    renderDetail("23");
    await flushAsync();

    expect(
      screen.getByRole("heading", { name: "動画が見つかりません" }),
    ).toBeInTheDocument();
  });

  it("不正なVideo IDでは詳細APIを呼ばない", () => {
    renderDetail("deleted");

    expect(
      screen.getByRole("heading", { name: "動画が見つかりません" }),
    ).toBeInTheDocument();
    expect(getVideoDetailMock).not.toHaveBeenCalled();
  });
});
