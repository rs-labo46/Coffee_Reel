import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  ownedVideoDetail,
  publicVideo,
  startUploadResponse,
} from "../tests/videoFixtures";
import type {
  OwnedVideoListResponse,
  PublicVideoListResponse,
  StartVideoUploadInput,
  VideoStateResponse,
} from "../types/video";
import { apiRequest } from "./client";
import {
  completeVideoUpload,
  deleteVideo,
  getMyVideo,
  getVideoDetail,
  listMyVideos,
  listReels,
  listSavedVideos,
  removeSavedVideo,
  republishVideo,
  saveVideo,
  setVideoPrivate,
  startVideoUpload,
} from "./video";

vi.mock("./client", () => ({
  apiRequest: vi.fn(),
}));

const apiRequestMock = vi.mocked(apiRequest);

const startInput: StartVideoUploadInput = {
  title: "ハンドドリップの蒸らし方",
  description: "30秒蒸らしてからゆっくり注ぎます",
  category: "brewing",
  file_content_type: "video/mp4",
  file_size_bytes: 1024,
};

const videoState: VideoStateResponse = {
  id: 10,
  processing_status: "ready",
  publish_status: "published",
};

const publicList: PublicVideoListResponse = {
  items: [publicVideo()],
  next_cursor: null,
  has_more: false,
};

const ownedList: OwnedVideoListResponse = {
  items: [],
  next_cursor: null,
  has_more: false,
};

describe("動画API", () => {
  beforeEach(() => {
    apiRequestMock.mockReset();
  });

  it("Idempotency-Keyと投稿入力を送信して投稿開始APIを呼ぶ", async () => {
    const response = startUploadResponse();
    apiRequestMock.mockResolvedValue(response);

    await expect(
      startVideoUpload(startInput, "idempotency-key-1"),
    ).resolves.toEqual(response);

    expect(apiRequestMock).toHaveBeenCalledWith("/videos", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": "idempotency-key-1",
      },
      body: JSON.stringify(startInput),
    });
  });

  it("Video IDをPathへ設定してUpload完了通知APIを呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(videoState);

    await expect(completeVideoUpload(10)).resolves.toEqual(videoState);

    expect(apiRequestMock).toHaveBeenCalledWith("/videos/10/upload-complete", {
      method: "POST",
    });
  });

  it("公開一覧のCursorをURLエンコードして取得する", async () => {
    apiRequestMock.mockResolvedValue(publicList);

    await expect(
      listReels({ limit: 50, cursor: "cursor+/=value" }),
    ).resolves.toEqual(publicList);

    expect(apiRequestMock).toHaveBeenCalledWith(
      "/videos?limit=50&cursor=cursor%2B%2F%3Dvalue",
      {
        method: "GET",
      },
    );
  });

  it("公開検索へTitle・Category・CursorをURL Queryとして送る", async () => {
    const controller = new AbortController();
    apiRequestMock.mockResolvedValue({
      ...publicList,
      result_type: "matched",
    });

    await listReels(
      {
        title: "ハンド ドリップ",
        category: "brewing",
        limit: 20,
        cursor: "opaque+/=cursor",
      },
      controller.signal,
    );

    expect(apiRequestMock).toHaveBeenCalledWith(
      "/videos?limit=20&title=%E3%83%8F%E3%83%B3%E3%83%89+%E3%83%89%E3%83%AA%E3%83%83%E3%83%97&category=brewing&cursor=opaque%2B%2F%3Dcursor",
      {
        method: "GET",
        signal: controller.signal,
      },
    );
  });

  it("公開詳細と自分の投稿詳細を正しいPathから取得する", async () => {
    const detail = publicVideo();
    const myDetail = ownedVideoDetail();

    apiRequestMock
      .mockResolvedValueOnce(detail)
      .mockResolvedValueOnce(myDetail);

    await expect(getVideoDetail(10)).resolves.toEqual(detail);
    await expect(getMyVideo(10)).resolves.toEqual(myDetail);

    expect(apiRequestMock).toHaveBeenNthCalledWith(1, "/videos/10", {
      method: "GET",
    });
    expect(apiRequestMock).toHaveBeenNthCalledWith(2, "/me/videos/10", {
      method: "GET",
    });
  });

  it("自分の投稿一覧と保存一覧へ既定limit 20を設定する", async () => {
    apiRequestMock
      .mockResolvedValueOnce(ownedList)
      .mockResolvedValueOnce(publicList);

    await listMyVideos();
    await listSavedVideos();

    expect(apiRequestMock).toHaveBeenNthCalledWith(1, "/me/videos?limit=20", {
      method: "GET",
    });
    expect(apiRequestMock).toHaveBeenNthCalledWith(
      2,
      "/me/saved-videos?limit=20",
      {
        method: "GET",
      },
    );
  });

  it("非公開と再公開APIをPATCHで呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(videoState);

    await setVideoPrivate(10);
    await republishVideo(10);

    expect(apiRequestMock).toHaveBeenNthCalledWith(1, "/me/videos/10/private", {
      method: "PATCH",
    });
    expect(apiRequestMock).toHaveBeenNthCalledWith(2, "/me/videos/10/publish", {
      method: "PATCH",
    });
  });

  it("保存・保存解除・動画削除をBodyなしで呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(undefined);

    await saveVideo(10);
    await removeSavedVideo(10);
    await deleteVideo(10);

    expect(apiRequestMock).toHaveBeenNthCalledWith(1, "/videos/10/saved", {
      method: "PUT",
    });
    expect(apiRequestMock).toHaveBeenNthCalledWith(2, "/videos/10/saved", {
      method: "DELETE",
    });
    expect(apiRequestMock).toHaveBeenNthCalledWith(3, "/me/videos/10", {
      method: "DELETE",
    });
  });
});
