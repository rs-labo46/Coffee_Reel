import { beforeEach, describe, expect, it, vi } from "vitest";

import type {
  AdminVideoDetailResponse,
  AdminVideoListResponse,
  AdminVideoStateResponse,
} from "../types/admin_video";
import {
  getAdminVideo,
  hideAdminVideo,
  listAdminVideos,
  restoreAdminVideo,
} from "./admin_video";
import { apiRequest } from "./client";

vi.mock("./client", () => ({
  apiRequest: vi.fn(),
}));

const apiRequestMock = vi.mocked(apiRequest);

const listResponse: AdminVideoListResponse = {
  items: [],
  next_cursor: null,
  has_more: false,
};

const detailResponse: AdminVideoDetailResponse = {
  id: 10,
  author: {
    id: 2,
    name: "投稿者",
    status: "active",
  },
  title: "ハンドドリップの基本",
  description: "抽出手順を説明します",
  category: "brewing",
  processing_status: "ready",
  publish_status: "published",
  playback_url: "https://storage.example/video",
  thumbnail_url: "https://storage.example/thumbnail",
  created_at: "2026-08-06T00:00:00Z",
  updated_at: "2026-08-06T01:00:00Z",
};

const stateResponse: AdminVideoStateResponse = {
  id: 10,
  processing_status: "ready",
  publish_status: "hidden",
  updated_at: "2026-08-06T02:00:00Z",
};

describe("管理者投稿API", () => {
  beforeEach(() => {
    apiRequestMock.mockReset();
  });

  it("Cursorなしで投稿一覧APIを呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(listResponse);

    await expect(listAdminVideos()).resolves.toEqual(listResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/videos?limit=20", {
      method: "GET",
    });
  });

  it("CursorをURLエンコードして次の投稿一覧を取得する", async () => {
    apiRequestMock.mockResolvedValue(listResponse);

    await listAdminVideos("cursor+/=value", 50);

    expect(apiRequestMock).toHaveBeenCalledWith(
      "/admin/videos?limit=50&cursor=cursor%2B%2F%3Dvalue",
      {
        method: "GET",
      },
    );
  });

  it("Video IDをPathへ設定して投稿詳細APIを呼ぶ", async () => {
    apiRequestMock.mockResolvedValue(detailResponse);

    await expect(getAdminVideo(10)).resolves.toEqual(detailResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/videos/10", {
      method: "GET",
    });
  });

  it("理由をJSONで送信して投稿を非公開にする", async () => {
    apiRequestMock.mockResolvedValue(stateResponse);

    await expect(
      hideAdminVideo(10, { reason: "利用規約違反を確認したため" }),
    ).resolves.toEqual(stateResponse);

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/videos/10/hide", {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        reason: "利用規約違反を確認したため",
      }),
    });
  });

  it("理由をJSONで送信して投稿の公開を再開する", async () => {
    apiRequestMock.mockResolvedValue({
      ...stateResponse,
      publish_status: "published",
    });

    await restoreAdminVideo(10, {
      reason: "確認が完了したため",
    });

    expect(apiRequestMock).toHaveBeenCalledWith("/admin/videos/10/restore", {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        reason: "確認が完了したため",
      }),
    });
  });
});
