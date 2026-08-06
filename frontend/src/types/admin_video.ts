import type { UserStatus } from "../types/user";
import type {
  CategoryCode,
  VideoProcessingStatus,
  VideoPublishStatus,
} from "../types/video";

export type AdminVideoAuthor = {
  id: number;
  name: string;
  status: UserStatus;
};

export type AdminVideoListItem = {
  id: number;
  author: AdminVideoAuthor;
  title: string;
  description: string;
  category: CategoryCode;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
  created_at: string;
  updated_at: string;
};

export type AdminVideoListResponse = {
  items: AdminVideoListItem[];
  next_cursor: string | null;
  has_more: boolean;
};

export type AdminVideoDetailResponse = AdminVideoListItem & {
  playback_url: string | null;
  thumbnail_url: string | null;
};

export type AdminVideoStateResponse = {
  id: number;
  processing_status: VideoProcessingStatus;
  publish_status: VideoPublishStatus;
  updated_at: string;
};

export type AdminVideoReasonInput = {
  reason: string;
};
