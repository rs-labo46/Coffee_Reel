import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import BookmarkIcon from "./BookmarkIcon";
import LikeButton from "./LikeButton";
import type { CategoryCode, PublicVideo, VideoLikeState } from "../types/video";

type ReelVideoProps = {
  video: PublicVideo;
  isActive: boolean;
  shouldPreload: boolean;
  isAuthenticated: boolean;
  isSaving: boolean;
  onVisibilityChange: (videoID: number, intersectionRatio: number) => void;
  onToggleSaved: (video: PublicVideo) => void;

  onLikeChange: (state: VideoLikeState) => void;
  onLikeNotFound: (videoID: number) => void;
  onLikeError: (error: unknown) => void;
};

// Category Codeをリール表示用の日本語へ変換
function categoryLabelOf(category: CategoryCode): string {
  switch (category) {
    case "brewing":
      return "抽出";
    case "roasting":
      return "焙煎";
    case "latte_art":
      return "ラテアート";
    case "beans":
      return "コーヒー豆";
    case "equipment":
      return "器具";
  }
}

// Browserの再生Promiseを安全に扱い失敗を画面へ反映
function playVideo(
  videoElement: HTMLVideoElement,
  onFailure: () => void,
): void {
  try {
    const playPromise = videoElement.play();

    if (playPromise !== undefined) {
      void playPromise.catch(onFailure);
    }
  } catch {
    onFailure();
  }
}

// 1件の縦動画、再生状態、保存操作、表示範囲監視を管理
export default function ReelVideo({
  video,
  isActive,
  shouldPreload,
  isAuthenticated,
  isSaving,
  onVisibilityChange,
  onToggleSaved,

  onLikeChange,
  onLikeNotFound,
  onLikeError,
}: ReelVideoProps) {
  const containerRef = useRef<HTMLElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [isMuted, setIsMuted] = useState<boolean>(true);
  const [playbackFailed, setPlaybackFailed] = useState<boolean>(false);
  const shouldLoadVideo = isActive || shouldPreload;

  // Viewport内の表示割合を親Pageへ通知
  useEffect(() => {
    const container = containerRef.current;

    if (container === null || typeof IntersectionObserver === "undefined") {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];

        if (entry !== undefined) {
          onVisibilityChange(
            video.id,
            entry.isIntersecting ? entry.intersectionRatio : 0,
          );
        }
      },
      {
        threshold: [0, 0.25, 0.5, 0.75, 1],
      },
    );

    observer.observe(container);

    return () => {
      observer.disconnect();
      onVisibilityChange(video.id, 0);
    };
  }, [onVisibilityChange, video.id]);

  // Active動画だけ再生し画面外の動画を停止
  useEffect(() => {
    const videoElement = videoRef.current;

    if (videoElement === null) {
      return;
    }

    if (!isActive) {
      videoElement.pause();
      return;
    }

    playVideo(videoElement, () => setPlaybackFailed(true));
  }, [isActive, shouldLoadVideo]);

  // 読込対象外へ移動した動画のMedia取得を解除
  useEffect(() => {
    const videoElement = videoRef.current;

    if (videoElement === null || shouldLoadVideo) {
      return;
    }

    videoElement.pause();
    videoElement.removeAttribute("src");
    videoElement.load();
  }, [shouldLoadVideo]);

  // 再生失敗後に利用者操作で再試行
  function handleRetryPlayback(): void {
    const videoElement = videoRef.current;

    if (videoElement === null) {
      return;
    }

    playVideo(videoElement, () => setPlaybackFailed(true));
  }

  // Muted状態を動画Elementと画面表示へ反映
  function handleToggleMuted(): void {
    setIsMuted((currentMuted) => !currentMuted);
  }

  return (
    <article
      ref={containerRef}
      className="relative mx-auto flex min-h-[calc(100dvh-5rem)] w-full max-w-5xl snap-start items-center justify-center px-3 py-5 sm:px-6"
      aria-label={`${video.title}のリール動画`}
    >
      <div className="relative aspect-[9/16] max-h-[calc(100dvh-7.5rem)] w-full max-w-[min(100%,30rem)] overflow-hidden rounded-[2rem] border border-white/10 bg-black shadow-2xl shadow-black/40">
        <video
          ref={videoRef}
          src={shouldLoadVideo ? video.playback_url : undefined}
          poster={video.thumbnail_url}
          muted={isMuted}
          playsInline
          loop
          preload={isActive ? "auto" : shouldPreload ? "metadata" : "none"}
          onCanPlay={() => {
            setPlaybackFailed(false);

            if (isActive && videoRef.current !== null) {
              playVideo(videoRef.current, () => setPlaybackFailed(true));
            }
          }}
          onError={() => setPlaybackFailed(true)}
          className="h-full w-full object-cover"
          aria-label={`${video.title}を再生`}
        />

        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/90 via-black/5 to-black/35"
        />

        {playbackFailed && isActive && (
          <div className="absolute inset-0 grid place-items-center bg-black/65 px-8 text-center">
            <div>
              <p className="text-sm font-black text-white">
                動画を再生できませんでした
              </p>
              <button
                type="button"
                onClick={handleRetryPlayback}
                className="mt-4 rounded-full bg-white px-5 py-2 text-xs font-black text-stone-950 transition hover:bg-stone-200"
              >
                再試行
              </button>
            </div>
          </div>
        )}

        <div className="absolute inset-x-0 bottom-0 z-10 p-5 sm:p-6">
          <div className="flex items-end justify-between gap-4">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-xs font-black">
                <span className="rounded-full bg-amber-300 px-3 py-1 text-stone-950">
                  {categoryLabelOf(video.category)}
                </span>
                <Link
                  to={`/videos/author/${video.author.id}`}
                  aria-label={`${video.author.name}の公開動画を見る`}
                  className="text-stone-200 transition hover:text-amber-200"
                >
                  {video.author.name}
                </Link>
              </div>

              <Link
                to={`/videos/${video.id}`}
                className="mt-3 block text-xl font-black leading-tight text-white transition hover:text-amber-200 sm:text-2xl"
              >
                {video.title}
              </Link>

              {video.description !== "" && (
                <p className="mt-2 line-clamp-3 text-sm leading-6 text-stone-200">
                  {video.description}
                </p>
              )}
            </div>

            <div className="flex shrink-0 flex-col gap-3">
              <button
                type="button"
                onClick={handleToggleMuted}
                className="grid h-11 w-11 place-items-center rounded-full border border-white/20 bg-black/50 text-white backdrop-blur-sm transition hover:bg-black/70"
                aria-label={isMuted ? "音声をオン" : "音声をオフ"}
              >
                {isMuted ? (
                  <svg
                    viewBox="0 0 24 24"
                    className="h-5 w-5"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.8"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M11 5 6.5 9H3v6h3.5l4.5 4V5Z" />
                    <path d="M4 4 20 20" />
                  </svg>
                ) : (
                  <svg
                    viewBox="0 0 24 24"
                    className="h-5 w-5"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.8"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <path d="M11 5 6.5 9H3v6h3.5l4.5 4V5Z" />
                    <path d="M15 9.5a4 4 0 0 1 0 5" />
                    <path d="M17.5 7a7.5 7.5 0 0 1 0 10" />
                  </svg>
                )}
              </button>

              <LikeButton
                videoID={video.id}
                likeCount={video.like_count}
                isLiked={video.is_liked}
                isAuthenticated={isAuthenticated}
                onChange={onLikeChange}
                onNotFound={() => onLikeNotFound(video.id)}
                onError={onLikeError}
                className="h-11 w-11 border-white/20 bg-black/50 backdrop-blur-sm hover:bg-black/70"
              />

              <button
                type="button"
                onClick={() => onToggleSaved(video)}
                disabled={isSaving}
                aria-pressed={video.is_saved}
                aria-busy={isSaving}
                aria-label={
                  isSaving
                    ? "保存を更新中"
                    : video.is_saved
                      ? "保存を解除"
                      : isAuthenticated
                        ? "保存"
                        : "ログインして保存"
                }
                className={`grid h-11 w-11 place-items-center rounded-full border bg-black/50 text-white backdrop-blur-sm transition hover:bg-black/70 disabled:cursor-not-allowed disabled:opacity-50 ${
                  video.is_saved
                    ? "border-amber-300/60 text-amber-300"
                    : "border-white/20"
                }`}
              >
                <BookmarkIcon filled={video.is_saved} />
              </button>
            </div>
          </div>
        </div>
      </div>
    </article>
  );
}
