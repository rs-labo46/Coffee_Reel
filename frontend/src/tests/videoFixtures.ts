import type { AuthContextValue } from "../auth/authContext";
import type {
  OwnedVideo,
  OwnedVideoDetail,
  PublicVideo,
  StartVideoUploadResponse,
  VideoUploadTarget,
} from "../types/video";

// 認証済み一般UserのContext値を生成
export function authenticatedUser(
  overrides: Partial<AuthContextValue> = {},
): AuthContextValue {
  return {
    user: {
      id: 1,
      name: "コーヒー太郎",
      email: "coffee@example.com",
      role: "user",
      status: "active",
    },
    accessToken: "access-token",
    isAuthenticated: true,
    isLoading: false,
    login: async () => undefined,
    logout: async () => undefined,
    ...overrides,
  };
}

// 未認証UserのContext値を生成
export function guestUser(
  overrides: Partial<AuthContextValue> = {},
): AuthContextValue {
  return {
    user: null,
    accessToken: null,
    isAuthenticated: false,
    isLoading: false,
    login: async () => undefined,
    logout: async () => undefined,
    ...overrides,
  };
}

// 公開動画ResponseのFixtureを生成
export function publicVideo(
  overrides: Partial<PublicVideo> = {},
): PublicVideo {
  return {
    id: 10,
    title: "ハンドドリップの蒸らし方",
    description: "30秒蒸らしてからゆっくり注ぎます",
    category: "brewing",
    author: {
      id: 1,
      name: "コーヒー太郎",
    },
    playback_url: "https://storage.example.com/video.mp4",
    thumbnail_url: "https://storage.example.com/thumbnail.jpg",
    is_saved: false,
    created_at: "2026-08-05T00:00:00Z",
    ...overrides,
  };
}

// 自分の投稿一覧ResponseのFixtureを生成
export function ownedVideo(
  overrides: Partial<OwnedVideo> = {},
): OwnedVideo {
  return {
    id: 10,
    title: "ハンドドリップの蒸らし方",
    category: "brewing",
    processing_status: "processing",
    publish_status: "private",
    thumbnail_url: null,
    created_at: "2026-08-05T00:00:00Z",
    updated_at: "2026-08-05T00:01:00Z",
    ...overrides,
  };
}

// 自分の投稿詳細ResponseのFixtureを生成
export function ownedVideoDetail(
  overrides: Partial<OwnedVideoDetail> = {},
): OwnedVideoDetail {
  return {
    id: 10,
    title: "ハンドドリップの蒸らし方",
    description: "30秒蒸らしてからゆっくり注ぎます",
    category: "brewing",
    processing_status: "processing",
    publish_status: "private",
    failure_code: null,
    playback_url: null,
    thumbnail_url: null,
    created_at: "2026-08-05T00:00:00Z",
    updated_at: "2026-08-05T00:01:00Z",
    ...overrides,
  };
}

// Storage直接Upload用TargetのFixtureを生成
export function uploadTarget(
  overrides: Partial<VideoUploadTarget> = {},
): VideoUploadTarget {
  return {
    method: "PUT",
    url: "https://storage.example.com/upload",
    headers: {
      "Content-Type": "video/mp4",
    },
    expires_at: "2026-08-05T00:10:00Z",
    ...overrides,
  };
}

// 投稿開始ResponseのFixtureを生成
export function startUploadResponse(
  overrides: Partial<StartVideoUploadResponse> = {},
): StartVideoUploadResponse {
  return {
    video: {
      id: 10,
      title: "ハンドドリップの蒸らし方",
      description: "30秒蒸らしてからゆっくり注ぎます",
      category: "brewing",
      processing_status: "uploading",
      publish_status: "private",
      upload_expires_at: "2026-08-05T00:10:00Z",
      created_at: "2026-08-05T00:00:00Z",
    },
    upload: uploadTarget(),
    ...overrides,
  };
}
