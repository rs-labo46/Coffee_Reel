import type {
  OwnedVideoDetail,
  OwnedVideoListResponse,
  PublicVideo,
  PublicVideoListResponse,
  PublicVideoSearchQuery,
  StartVideoUploadInput,
  StartVideoUploadResponse,
  VideoListQuery,
  VideoStateResponse,
} from "../types/video";
import { apiRequest } from "./client";

const videosPath = "/videos";
const myVideosPath = "/me/videos";
const savedVideosPath = "/me/saved-videos";
const defaultVideoListLimit = 20;

export function startVideoUpload(
  input: StartVideoUploadInput,
  idempotencyKey: string,
): Promise<StartVideoUploadResponse> {
  return apiRequest<StartVideoUploadResponse>(videosPath, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
    },
    body: JSON.stringify(input),
  });
}

export function completeVideoUpload(
  videoID: number,
): Promise<VideoStateResponse> {
  return apiRequest<VideoStateResponse>(
    `${videosPath}/${videoID}/upload-complete`,
    {
      method: "POST",
    },
  );
}

export function getVideoDetail(videoID: number): Promise<PublicVideo> {
  return apiRequest<PublicVideo>(`${videosPath}/${videoID}`, {
    method: "GET",
  });
}

export function listReels(
  query: PublicVideoSearchQuery = {},
  signal?: AbortSignal,
): Promise<PublicVideoListResponse> {
  const init: RequestInit = {
    method: "GET",
  };

  if (signal !== undefined) {
    init.signal = signal;
  }

  return apiRequest<PublicVideoListResponse>(
    buildPublicListPath(videosPath, query),
    init,
  );
}

export function listMyVideos(
  query: VideoListQuery = {},
): Promise<OwnedVideoListResponse> {
  return apiRequest<OwnedVideoListResponse>(
    buildListPath(myVideosPath, query),
    {
      method: "GET",
    },
  );
}

export function getMyVideo(videoID: number): Promise<OwnedVideoDetail> {
  return apiRequest<OwnedVideoDetail>(`${myVideosPath}/${videoID}`, {
    method: "GET",
  });
}

export function setVideoPrivate(videoID: number): Promise<VideoStateResponse> {
  return changeVideoPublishStatus(videoID, "private");
}

export function republishVideo(videoID: number): Promise<VideoStateResponse> {
  return changeVideoPublishStatus(videoID, "publish");
}

export function deleteVideo(videoID: number): Promise<void> {
  return apiRequest<void>(`${myVideosPath}/${videoID}`, {
    method: "DELETE",
  });
}

export function saveVideo(videoID: number): Promise<void> {
  return apiRequest<void>(`${videosPath}/${videoID}/saved`, {
    method: "PUT",
  });
}

export function removeSavedVideo(videoID: number): Promise<void> {
  return apiRequest<void>(`${videosPath}/${videoID}/saved`, {
    method: "DELETE",
  });
}

export function listSavedVideos(
  query: VideoListQuery = {},
): Promise<PublicVideoListResponse> {
  return apiRequest<PublicVideoListResponse>(
    buildListPath(savedVideosPath, query),
    {
      method: "GET",
    },
  );
}

function changeVideoPublishStatus(
  videoID: number,
  action: "private" | "publish",
): Promise<VideoStateResponse> {
  return apiRequest<VideoStateResponse>(
    `${myVideosPath}/${videoID}/${action}`,
    {
      method: "PATCH",
    },
  );
}

function buildPublicListPath(
  path: string,
  query: PublicVideoSearchQuery,
): string {
  const searchParams = new URLSearchParams({
    limit: String(query.limit ?? defaultVideoListLimit),
  });

  if (query.title !== undefined && query.title !== "") {
    searchParams.set("title", query.title);
  }

  if (query.category !== undefined) {
    searchParams.set("category", query.category);
  }

  if (query.author_id !== undefined) {
    searchParams.set("author_id", String(query.author_id));
  }

  if (query.cursor !== undefined && query.cursor !== "") {
    searchParams.set("cursor", query.cursor);
  }

  return `${path}?${searchParams.toString()}`;
}

function buildListPath(path: string, query: VideoListQuery): string {
  const searchParams = new URLSearchParams({
    limit: String(query.limit ?? defaultVideoListLimit),
  });

  if (query.cursor !== undefined && query.cursor !== "") {
    searchParams.set("cursor", query.cursor);
  }

  return `${path}?${searchParams.toString()}`;
}
