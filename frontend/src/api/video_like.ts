import type { VideoLikeState } from "../types/video";
import { apiRequest } from "./client";

const videosPath = "/videos";

export function likeVideo(videoID: number): Promise<VideoLikeState> {
  return apiRequest<VideoLikeState>(`${videosPath}/${videoID}/like`, {
    method: "PUT",
  });
}

export function unlikeVideo(videoID: number): Promise<VideoLikeState> {
  return apiRequest<VideoLikeState>(`${videosPath}/${videoID}/like`, {
    method: "DELETE",
  });
}
