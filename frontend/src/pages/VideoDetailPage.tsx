import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";

import { ApiClientError } from "../api/client";
import { getVideoDetail, removeSavedVideo, saveVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import LikeButton from "../components/LikeButton";
import type { CategoryCode, PublicVideo, VideoLikeState } from "../types/video";

const initialDetailPromises = new Map<
  string,
  ReturnType<typeof getVideoDetail>
>();

// URL Path Parameterを正の安全なVideo IDへ変換
function parseVideoID(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }

  const videoID = Number(value);

  return Number.isSafeInteger(videoID) ? videoID : null;
}

// Category Codeを詳細画面用の日本語へ変換
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

// ISO日時を日本語の画面表示へ変換
function formatDate(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "long",
    timeStyle: "short",
  }).format(date);
}

// React StrictModeの再実行中に認証状態別の初回詳細Requestを共有
function requestInitialDetail(
  requestKey: string,
  videoID: number,
): ReturnType<typeof getVideoDetail> {
  const key = `${requestKey}:${videoID}`;
  const existingPromise = initialDetailPromises.get(key);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = getVideoDetail(videoID).finally(() => {
    initialDetailPromises.delete(key);
  });
  initialDetailPromises.set(key, request);

  return request;
}

// API Errorを動画詳細画面用Messageへ変換
function errorViewOf(error: unknown): {
  message: string;
  requestID: string;
} {
  if (error instanceof ApiClientError) {
    if (error.status === 404) {
      return {
        message: "動画が見つかりません",
        requestID: error.requestId,
      };
    }

    return {
      message: error.message,
      requestID: error.requestId,
    };
  }

  return {
    message: "動画詳細を取得できませんでした",
    requestID: "",
  };
}

// 公開動画詳細、再生、保存状態、いいね状態を管理
export default function VideoDetailPage() {
  const { video_id: rawVideoID } = useParams<{ video_id: string }>();
  const videoID = parseVideoID(rawVideoID);
  const location = useLocation();
  const navigate = useNavigate();
  const { isAuthenticated, isLoading: isAuthLoading, user } = useAuth();

  const requestKey =
    isAuthenticated && user !== null ? `user:${user.id}` : "guest";
  const requestIdentity = videoID === null ? null : `${requestKey}:${videoID}`;

  const saveInFlightRef = useRef(false);

  const [video, setVideo] = useState<PublicVideo | null>(null);
  const [resolvedIdentity, setResolvedIdentity] = useState<string | null>(null);
  const [failedIdentity, setFailedIdentity] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");

  const visibleVideo = resolvedIdentity === requestIdentity ? video : null;
  const isLoading =
    isAuthLoading ||
    (requestIdentity !== null &&
      resolvedIdentity !== requestIdentity &&
      failedIdentity !== requestIdentity);

  // 認証復元後に公開動画詳細を取得
  useEffect(() => {
    if (isAuthLoading || videoID === null || requestIdentity === null) {
      return;
    }

    let isActive = true;

    requestInitialDetail(requestKey, videoID)
      .then((response) => {
        if (!isActive) {
          return;
        }

        setVideo(response);
        setResolvedIdentity(requestIdentity);
        setFailedIdentity(null);
        setErrorMessage("");
        setRequestID("");
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        const errorView = errorViewOf(error);
        setVideo(null);
        setResolvedIdentity(null);
        setFailedIdentity(requestIdentity);
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      });

    return () => {
      isActive = false;
    };
  }, [isAuthLoading, requestIdentity, requestKey, videoID]);

  // 未認証時はLoginへ移動し認証済み時は保存状態を切替
  async function handleToggleSaved(): Promise<void> {
    if (visibleVideo === null || saveInFlightRef.current) {
      return;
    }

    if (!isAuthenticated) {
      navigate("/login", {
        state: { from: `${location.pathname}${location.search}` },
      });
      return;
    }

    const targetVideoID = visibleVideo.id;
    const nextSaved = !visibleVideo.is_saved;

    saveInFlightRef.current = true;
    setIsSaving(true);
    setErrorMessage("");
    setRequestID("");

    try {
      if (visibleVideo.is_saved) {
        await removeSavedVideo(targetVideoID);
      } else {
        await saveVideo(targetVideoID);
      }

      setVideo((currentVideo) =>
        currentVideo === null || currentVideo.id !== targetVideoID
          ? currentVideo
          : {
              ...currentVideo,
              is_saved: nextSaved,
            },
      );
    } catch (error: unknown) {
      const errorView = errorViewOf(error);

      if (
        error instanceof ApiClientError &&
        error.status === 404 &&
        requestIdentity !== null
      ) {
        setVideo(null);
        setResolvedIdentity(null);
        setFailedIdentity(requestIdentity);
      }

      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      saveInFlightRef.current = false;
      setIsSaving(false);
    }
  }

  // PUT・DELETEのResponseをそのまま対象VideoのLike状態へ反映
  function handleLikeChange(state: VideoLikeState): void {
    setVideo((currentVideo) =>
      currentVideo === null || currentVideo.id !== state.video_id
        ? currentVideo
        : {
            ...currentVideo,
            like_count: state.like_count,
            is_liked: state.is_liked,
          },
    );
    setErrorMessage("");
    setRequestID("");
  }

  function handleLikeNotFound(): void {
    if (requestIdentity === null) {
      return;
    }

    setVideo(null);
    setResolvedIdentity(null);
    setFailedIdentity(requestIdentity);
    setErrorMessage("動画が見つかりません");
    setRequestID("");
  }

  function handleLikeError(error: unknown): void {
    const errorView = errorViewOf(error);
    setErrorMessage(errorView.message);
    setRequestID(errorView.requestID);
  }

  if (videoID === null) {
    return (
      <main className="grid min-h-dvh place-items-center bg-[#100b08] px-4 py-10 text-stone-100">
        <section className="w-full max-w-lg rounded-[2rem] border border-white/10 bg-white/[0.05] p-8 text-center shadow-2xl shadow-black/30">
          <p className="text-xs font-black tracking-[0.22em] text-red-300 uppercase">
            Video unavailable
          </p>
          <h1 className="mt-4 text-3xl font-black text-white">
            動画が見つかりません
          </h1>
          <Link
            to="/"
            className="mt-7 inline-flex min-h-12 items-center justify-center rounded-2xl bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200"
          >
            リールへ戻る
          </Link>
        </section>
      </main>
    );
  }

  if (isLoading) {
    return (
      <main className="grid min-h-dvh place-items-center bg-[#100b08] px-4 text-stone-100">
        <div
          className="flex items-center gap-3 text-sm font-bold text-stone-300"
          role="status"
        >
          <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
          動画を読み込んでいます
        </div>
      </main>
    );
  }

  if (visibleVideo === null) {
    return (
      <main className="grid min-h-dvh place-items-center bg-[#100b08] px-4 py-10 text-stone-100">
        <section className="w-full max-w-lg rounded-[2rem] border border-white/10 bg-white/[0.05] p-8 text-center shadow-2xl shadow-black/30">
          <p className="text-xs font-black tracking-[0.22em] text-red-300 uppercase">
            Video unavailable
          </p>
          <h1 className="mt-4 text-3xl font-black text-white">
            {errorMessage || "動画が見つかりません"}
          </h1>
          {requestID !== "" && (
            <p className="mt-4 break-all text-xs font-bold text-stone-500">
              Request ID: {requestID}
            </p>
          )}
          <Link
            to="/"
            className="mt-7 inline-flex min-h-12 items-center justify-center rounded-2xl bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200"
          >
            リールへ戻る
          </Link>
        </section>
      </main>
    );
  }

  const detail = visibleVideo;

  return (
    <main className="min-h-dvh bg-[#100b08] px-4 py-6 text-stone-100 sm:px-6 lg:px-10">
      <div className="mx-auto w-full max-w-6xl">
        <header className="flex flex-wrap items-center justify-between gap-4">
          <Link
            to="/"
            className="text-sm font-black tracking-[0.18em] text-amber-300 uppercase"
          >
            Coffee Reel
          </Link>

          <nav
            className="flex flex-wrap items-center gap-3"
            aria-label="動画画面ナビゲーション"
          >
            <Link
              to="/"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              リール
            </Link>
            {/* ---追加--- */}
            <Link
              to="/search"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              検索
            </Link>
            {/* ---追加--- */}
            {isAuthenticated && (
              <Link
                to="/me/saved-videos"
                className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
              >
                保存一覧
              </Link>
            )}
          </nav>
        </header>

        {errorMessage !== "" && (
          <div
            className="mt-6 rounded-2xl border border-red-300/25 bg-red-400/10 px-4 py-3 text-sm font-bold text-red-100"
            role="alert"
          >
            <p>{errorMessage}</p>
            {requestID !== "" && (
              <p className="mt-1 break-all text-xs text-red-200/70">
                Request ID: {requestID}
              </p>
            )}
          </div>
        )}

        <article className="mt-6 grid gap-6 lg:grid-cols-[minmax(0,0.82fr)_minmax(320px,0.58fr)] lg:items-start">
          <section className="overflow-hidden rounded-[2rem] border border-white/10 bg-black shadow-2xl shadow-black/30">
            <video
              src={detail.playback_url}
              poster={detail.thumbnail_url}
              controls
              playsInline
              preload="metadata"
              className="aspect-[9/16] max-h-[78dvh] w-full bg-black object-contain"
            >
              動画を再生できません。
            </video>
          </section>

          <section className="rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 shadow-xl shadow-black/20 sm:p-8">
            <div className="flex flex-wrap items-center gap-3">
              <span className="rounded-full bg-amber-300 px-3 py-1 text-xs font-black text-stone-950">
                {categoryLabelOf(detail.category)}
              </span>
              <time className="text-xs font-bold text-stone-500">
                {formatDate(detail.created_at)}
              </time>
            </div>

            <h1 className="mt-5 text-3xl font-black leading-tight text-white sm:text-4xl">
              {detail.title}
            </h1>

            <p className="mt-4 text-sm font-black text-amber-200">
              {detail.author.name}
            </p>

            {detail.description !== "" && (
              <p className="mt-5 whitespace-pre-wrap break-words text-sm leading-7 text-stone-300">
                {detail.description}
              </p>
            )}

            {/* ---追加--- */}
            <div className="mt-7 grid gap-3 sm:grid-cols-2">
              <LikeButton
                videoID={detail.id}
                likeCount={detail.like_count}
                isLiked={detail.is_liked}
                isAuthenticated={isAuthenticated}
                onChange={handleLikeChange}
                onNotFound={handleLikeNotFound}
                onError={handleLikeError}
                className="min-h-12 w-full rounded-2xl"
              />

              <button
                type="button"
                onClick={() => void handleToggleSaved()}
                disabled={isSaving}
                className="inline-flex min-h-12 w-full items-center justify-center rounded-2xl bg-amber-300 px-5 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {isSaving
                  ? "更新中"
                  : detail.is_saved
                    ? "保存を解除"
                    : "動画を保存"}
              </button>
            </div>
            {/* ---追加--- */}

            <div className="mt-6 overflow-hidden rounded-[1.5rem] border border-white/10 bg-black/20">
              <img
                src={detail.thumbnail_url}
                alt={`${detail.title}のサムネイル`}
                className="aspect-[9/16] max-h-72 w-full object-cover"
              />
            </div>
          </section>
        </article>
      </div>
    </main>
  );
}
