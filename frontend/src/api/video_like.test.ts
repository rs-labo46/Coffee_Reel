import { beforeEach, describe, expect, it, vi } from "vitest";

import type { VideoLikeState } from "../types/video";
import { apiRequest } from "./client";
import { likeVideo, unlikeVideo } from "./video_like";

vi.mock("./client", () => ({
  apiRequest: vi.fn(),
}));

const apiRequestMock = vi.mocked(apiRequest);

const likedState: VideoLikeState = {
  video_id: 10,
  like_count: 4,
  is_liked: true,
};

const unlikedState: VideoLikeState = {
  video_id: 10,
  like_count: 3,
  is_liked: false,
};

describe("動画いいねAPI", () => {
  beforeEach(() => {
    apiRequestMock.mockReset();
  });

  it("PUT /videos/:video_id/likeでいいねする", async () => {
    apiRequestMock.mockResolvedValue(likedState);

    await expect(likeVideo(10)).resolves.toEqual(likedState);

    expect(apiRequestMock).toHaveBeenCalledWith("/videos/10/like", {
      method: "PUT",
    });
  });

  it("DELETE /videos/:video_id/likeでいいね解除する", async () => {
    apiRequestMock.mockResolvedValue(unlikedState);

    await expect(unlikeVideo(10)).resolves.toEqual(unlikedState);

    expect(apiRequestMock).toHaveBeenCalledWith("/videos/10/like", {
      method: "DELETE",
    });
  });
});
