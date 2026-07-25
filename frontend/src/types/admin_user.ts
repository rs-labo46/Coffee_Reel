import type { UserStatus } from "./user";

export type AdminUserListItem = {
  id: number;
  name: string;
  email: string;
  status: UserStatus;
  created_at: string;
};

export type AdminUserListResponse = {
  items: AdminUserListItem[];
  next_cursor: string | null;
  has_more: boolean;
};

export type AdminUserVideo = {
  id: number;
  title: string;
  processing_status: string;
  publish_status: string;
  created_at: string;
};

export type AdminUserDetailResponse = {
  id: number;
  name: string;
  email: string;
  status: UserStatus;
  created_at: string;
  videos: AdminUserVideo[];
};

export type AdminUserStatusResponse = {
  id: number;
  status: UserStatus;
  updated_at: string;
};

export type AdminUserReasonInput = {
  reason: string;
};
