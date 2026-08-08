export type CategoryCode =
  | "brewing"
  | "roasting"
  | "latte_art"
  | "beans"
  | "equipment";

export type VideoProcessingStatus =
  | "uploading"
  | "expired"
  | "uploaded"
  | "processing"
  | "ready"
  | "failed";

export type VideoPublishStatus = "private" | "published" | "hidden";

export type VideoFailureCode =
  | "invalid_format"
  | "video_corrupt"
  | "duration_exceeded"
  | "size_exceeded"
  | "resolution_exceeded"
  | "invalid_aspect_ratio"
  | "frame_rate_exceeded"
  | "video_track_missing"
  | "processing_failed";

export type VideoFileContentType = "video/mp4" | "video/quicktime";

export type StartVideoUploadInput = {
  title: string;
  description: string;
  category: CategoryCode;
  file_content_type: VideoFileContentType;
  file_size_bytes: number;
};

export type StartVideoUploadVideo = {
  id: number;
  title: string;
  description: string;
  category: CategoryCode;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
  upload_expires_at: string;
  created_at: string;
};

export type VideoUploadTarget = {
  method: "PUT";
  url: string;
  headers: {
    "Content-Type": VideoFileContentType;
  };
  expires_at: string;
};

export type StartVideoUploadResponse = {
  video: StartVideoUploadVideo;
  upload: VideoUploadTarget;
};

export type VideoStateResponse = {
  id: number;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
};

export type VideoAuthor = {
  id: number;
  name: string;
};

export type PublicSearchResultType = "all" | "matched" | "similar";

export type PublicVideo = {
  id: number;
  title: string;
  description: string;
  category: CategoryCode;
  author: VideoAuthor;
  playback_url: string;
  thumbnail_url: string;
  like_count: number;
  is_liked: boolean;
  is_saved: boolean;
  created_at: string;
};

export type PublicVideoListResponse = {
  items: PublicVideo[];
  next_cursor: string | null;
  has_more: boolean;
  result_type?: PublicSearchResultType;
};

export type PublicVideoSearchQuery = {
  title?: string;
  category?: CategoryCode;
  limit?: number;
  cursor?: string;
};

export type VideoLikeState = {
  video_id: number;
  like_count: number;
  is_liked: boolean;
};

export type OwnedVideo = {
  id: number;
  title: string;
  category: CategoryCode;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
  thumbnail_url: string | null;
  created_at: string;
  updated_at: string;
};

export type OwnedVideoListResponse = {
  items: OwnedVideo[];
  next_cursor: string | null;
  has_more: boolean;
};

export type OwnedVideoDetail = {
  id: number;
  title: string;
  description: string;
  category: CategoryCode;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
  failure_code: VideoFailureCode | null;
  playback_url: string | null;
  thumbnail_url: string | null;
  created_at: string;
  updated_at: string;
};

export type VideoListQuery = {
  limit?: number;
  cursor?: string;
};
