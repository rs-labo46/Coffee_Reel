import type {
  AdminVideoDetailResponse,
  AdminVideoListResponse,
  AdminVideoReasonInput,
  AdminVideoStateResponse,
} from "../types/admin_video";
import { apiRequest } from "./client";

const adminVideosPath = "/admin/videos";

export function listAdminVideos(
  cursor: string | null = null,
  limit = 20,
): Promise<AdminVideoListResponse> {
  const query = new URLSearchParams({ limit: String(limit) });

  if (cursor !== null) {
    query.set("cursor", cursor);
  }

  return apiRequest<AdminVideoListResponse>(
    `${adminVideosPath}?${query.toString()}`,
    {
      method: "GET",
    },
  );
}

export function getAdminVideo(
  videoID: number,
): Promise<AdminVideoDetailResponse> {
  return apiRequest<AdminVideoDetailResponse>(`${adminVideosPath}/${videoID}`, {
    method: "GET",
  });
}

export function hideAdminVideo(
  videoID: number,
  input: AdminVideoReasonInput,
): Promise<AdminVideoStateResponse> {
  return changeAdminVideoState(videoID, "hide", input);
}

export function restoreAdminVideo(
  videoID: number,
  input: AdminVideoReasonInput,
): Promise<AdminVideoStateResponse> {
  return changeAdminVideoState(videoID, "restore", input);
}

function changeAdminVideoState(
  videoID: number,
  action: "hide" | "restore",
  input: AdminVideoReasonInput,
): Promise<AdminVideoStateResponse> {
  return apiRequest<AdminVideoStateResponse>(
    `${adminVideosPath}/${videoID}/${action}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(input),
    },
  );
}
