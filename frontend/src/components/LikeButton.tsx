import { useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router";

import { ApiClientError } from "../api/client";
import { likeVideo, unlikeVideo } from "../api/video_like";
import type { VideoLikeState } from "../types/video";

type LikeButtonProps = {
  videoID: number;
  likeCount: number;
  isLiked: boolean;
  isAuthenticated: boolean;
  onChange: (state: VideoLikeState) => void;
  onNotFound?: () => void;
  onError?: (error: unknown) => void;
  className?: string;
};

function currentRelativePath(pathname: string, search: string): string {
  const path = `${pathname}${search}`;

  if (!path.startsWith("/") || path.startsWith("//")) {
    return "/";
  }

  return path;
}

// 一覧・検索・詳細で共通利用するいいね操作Button
export default function LikeButton({
  videoID,
  likeCount,
  isLiked,
  isAuthenticated,
  onChange,
  onNotFound,
  onError,
  className = "",
}: LikeButtonProps) {
  const navigate = useNavigate();
  const location = useLocation();
  const requestInFlightRef = useRef(false);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  async function handleClick(): Promise<void> {
    if (!isAuthenticated) {
      const redirect = currentRelativePath(location.pathname, location.search);
      const searchParams = new URLSearchParams({ redirect });
      navigate(`/login?${searchParams.toString()}`);
      return;
    }

    if (requestInFlightRef.current) {
      return;
    }

    requestInFlightRef.current = true;
    setIsSubmitting(true);

    try {
      const state = isLiked
        ? await unlikeVideo(videoID)
        : await likeVideo(videoID);

      onChange(state);
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 404) {
        onNotFound?.();
        return;
      }

      onError?.(error);
    } finally {
      requestInFlightRef.current = false;
      setIsSubmitting(false);
    }
  }

  return (
    <button
      type="button"
      onClick={() => void handleClick()}
      disabled={isSubmitting}
      aria-pressed={isLiked}
      aria-label={
        isAuthenticated
          ? `${isLiked ? "いいねを解除" : "いいね"} ${likeCount}件`
          : `ログインしていいね ${likeCount}件`
      }
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-full border border-white/15 bg-black/20 px-4 py-2 text-sm font-black text-stone-100 transition hover:border-pink-300/50 hover:bg-pink-400/10 disabled:cursor-not-allowed disabled:opacity-60 ${className}`}
    >
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        className={`h-4 w-4 transition ${
          isLiked ? "text-pink-400" : "text-stone-100"
        }`}
        fill={isLiked ? "currentColor" : "none"}
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.6l-1-1a5.5 5.5 0 0 0-7.8 7.8l1 1L12 21l7.8-7.6 1-1a5.5 5.5 0 0 0 0-7.8Z" />
      </svg>

      <span>{isSubmitting ? "更新中" : "いいね"}</span>

      <span aria-hidden="true">{likeCount}</span>
    </button>
  );
}
