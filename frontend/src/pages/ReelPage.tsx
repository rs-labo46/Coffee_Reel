import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";

import { ApiClientError } from "../api/client";
import { listReels, removeSavedVideo, saveVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import BookmarkIcon from "../components/BookmarkIcon";
import ReelVideo from "../components/ReelVideo";
import type { PublicVideo, VideoLikeState } from "../types/video";

const initialReelsPromises = new Map<string, ReturnType<typeof listReels>>();

// React StrictModeの再実行中に認証状態別の初回一覧Requestを共有
function requestInitialReels(requestKey: string): ReturnType<typeof listReels> {
  const existingPromise = initialReelsPromises.get(requestKey);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = listReels().finally(() => {
    initialReelsPromises.delete(requestKey);
  });
  initialReelsPromises.set(requestKey, request);

  return request;
}

// Cursor追加取得結果をID重複なしで結合
function appendReels(
  currentVideos: PublicVideo[],
  nextVideos: PublicVideo[],
): PublicVideo[] {
  const existingIDs = new Set(currentVideos.map((video) => video.id));

  return [
    ...currentVideos,
    ...nextVideos.filter((video) => !existingIDs.has(video.id)),
  ];
}

// API Errorをリール画面用Messageへ変換
function errorViewOf(
  error: unknown,
  fallbackMessage: string,
): {
  message: string;
  requestID: string;
} {
  if (error instanceof ApiClientError) {
    return {
      message: error.message,
      requestID: error.requestId,
    };
  }

  return {
    message: fallbackMessage,
    requestID: "",
  };
}

// 公開リール、Active動画、次Cursor、保存状態を管理
export default function ReelPage() {
  const navigate = useNavigate();
  const { isAuthenticated, isLoading: isAuthLoading, logout, user } = useAuth();
  const visibilityRatiosRef = useRef<Map<number, number>>(new Map());
  const logoutInFlightRef = useRef(false);
  const loadMoreInFlightRef = useRef(false);
  const reelListRef = useRef<HTMLElement | null>(null);
  const loadMoreSentinelRef = useRef<HTMLDivElement | null>(null);

  const [videos, setVideos] = useState<PublicVideo[]>([]);
  const [activeVideoID, setActiveVideoID] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isInitialLoading, setIsInitialLoading] = useState<boolean>(true);
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false);
  const [savingVideoIDs, setSavingVideoIDs] = useState<Set<number>>(new Set());
  const [isLoggingOut, setIsLoggingOut] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");

  // 認証復元後に公開リールの先頭Pageを取得
  useEffect(() => {
    if (isAuthLoading) {
      return;
    }

    let isActive = true;
    visibilityRatiosRef.current.clear();

    const requestKey =
      isAuthenticated && user !== null ? `user:${user.id}` : "guest";

    requestInitialReels(requestKey)
      .then((response) => {
        if (!isActive) {
          return;
        }

        setVideos(response.items);
        setActiveVideoID(response.items[0]?.id ?? null);
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        const errorView = errorViewOf(
          error,
          "公開リールを取得できませんでした",
        );
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      })
      .finally(() => {
        if (isActive) {
          setIsInitialLoading(false);
        }
      });

    return () => {
      isActive = false;
    };
  }, [isAuthLoading, isAuthenticated, user]);

  // 各動画の表示割合から最も画面内にある1件をActiveへ変更
  const handleVisibilityChange = useCallback(
    (videoID: number, intersectionRatio: number): void => {
      if (intersectionRatio <= 0) {
        visibilityRatiosRef.current.delete(videoID);
      } else {
        visibilityRatiosRef.current.set(videoID, intersectionRatio);
      }

      let nextActiveVideoID: number | null = null;
      let highestRatio = 0;

      for (const [candidateID, ratio] of visibilityRatiosRef.current) {
        if (ratio > highestRatio) {
          highestRatio = ratio;
          nextActiveVideoID = candidateID;
        }
      }

      if (highestRatio >= 0.5) {
        setActiveVideoID((currentVideoID) =>
          currentVideoID === nextActiveVideoID
            ? currentVideoID
            : nextActiveVideoID,
        );
      }
    },
    [],
  );

  // 次Cursorを使って公開リールを追加取得
  const loadMore = useCallback(async (): Promise<void> => {
    if (!hasMore || nextCursor === null || loadMoreInFlightRef.current) {
      return;
    }

    loadMoreInFlightRef.current = true;
    setIsLoadingMore(true);
    setErrorMessage("");
    setRequestID("");

    try {
      const response = await listReels({ cursor: nextCursor });
      setVideos((currentVideos) => appendReels(currentVideos, response.items));
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more);
    } catch (error: unknown) {
      const errorView = errorViewOf(error, "次のリールを取得できませんでした");
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      loadMoreInFlightRef.current = false;
      setIsLoadingMore(false);
    }
  }, [hasMore, nextCursor]);

  const activeVideoIndex = videos.findIndex(
    (video) => video.id === activeVideoID,
  );

  // 一覧末尾のSentinel表示時に次Cursorを取得
  useEffect(() => {
    const sentinel = loadMoreSentinelRef.current;
    const reelList = reelListRef.current;

    if (
      sentinel === null ||
      reelList === null ||
      !hasMore ||
      typeof IntersectionObserver === "undefined"
    ) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          void loadMore();
        }
      },
      {
        root: reelList,
        rootMargin: "100% 0px",
      },
    );

    observer.observe(sentinel);

    return () => {
      observer.disconnect();
    };
  }, [hasMore, loadMore]);

  // 未認証時はLoginへ移動し認証済み時は保存状態を切替
  function handleLikeChange(state: VideoLikeState): void {
    setVideos((currentVideos) =>
      currentVideos.map((video) =>
        video.id === state.video_id
          ? {
              ...video,
              like_count: state.like_count,
              is_liked: state.is_liked,
            }
          : video,
      ),
    );
  }

  function handleLikeNotFound(videoID: number): void {
    visibilityRatiosRef.current.delete(videoID);
    setVideos((currentVideos) =>
      currentVideos.filter((video) => video.id !== videoID),
    );
    setActiveVideoID((currentVideoID) =>
      currentVideoID === videoID ? null : currentVideoID,
    );
  }

  function handleLikeError(error: unknown): void {
    const errorView = errorViewOf(error, "いいね状態を更新できませんでした");
    setErrorMessage(errorView.message);
    setRequestID(errorView.requestID);
  }

  async function handleToggleSaved(video: PublicVideo): Promise<void> {
    if (!isAuthenticated) {
      navigate("/login", { state: { from: "/" } });
      return;
    }

    if (savingVideoIDs.has(video.id)) {
      return;
    }

    setSavingVideoIDs((currentIDs) => {
      const nextIDs = new Set(currentIDs);
      nextIDs.add(video.id);
      return nextIDs;
    });
    setErrorMessage("");
    setRequestID("");

    try {
      if (video.is_saved) {
        await removeSavedVideo(video.id);
      } else {
        await saveVideo(video.id);
      }

      setVideos((currentVideos) =>
        currentVideos.map((currentVideo) =>
          currentVideo.id === video.id
            ? {
                ...currentVideo,
                is_saved: !video.is_saved,
              }
            : currentVideo,
        ),
      );
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 404) {
        visibilityRatiosRef.current.delete(video.id);
        setVideos((currentVideos) =>
          currentVideos.filter((currentVideo) => currentVideo.id !== video.id),
        );
        setActiveVideoID((currentVideoID) =>
          currentVideoID === video.id ? null : currentVideoID,
        );
      } else {
        const errorView = errorViewOf(
          error,
          video.is_saved
            ? "動画の保存を解除できませんでした"
            : "動画を保存できませんでした",
        );
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      }
    } finally {
      setSavingVideoIDs((currentIDs) => {
        const nextIDs = new Set(currentIDs);
        nextIDs.delete(video.id);
        return nextIDs;
      });
    }
  }

  // Headerからログアウトし、認証情報を破棄してLogin画面へ移動
  async function handleLogout(): Promise<void> {
    if (logoutInFlightRef.current) {
      return;
    }

    logoutInFlightRef.current = true;
    setIsLoggingOut(true);
    setErrorMessage("");
    setRequestID("");

    try {
      await logout();
      navigate("/login", { replace: true });
    } catch (error: unknown) {
      const errorView = errorViewOf(error, "ログアウトできませんでした");
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      logoutInFlightRef.current = false;
      setIsLoggingOut(false);
    }
  }

  return (
    <main className="min-h-dvh bg-[#100b08] text-stone-100">
      <header className="sticky top-0 z-30 border-b border-white/10 bg-[#100b08]/90 px-4 py-3 backdrop-blur-xl sm:px-6">
        <div className="mx-auto flex w-full max-w-6xl items-center justify-between gap-4">
          <Link
            to="/"
            className="text-xs font-black tracking-[0.18em] text-amber-300 uppercase sm:text-sm sm:tracking-[0.2em]"
          >
            Coffee Reel
          </Link>

          <nav
            className="flex flex-wrap items-center justify-end gap-1 sm:gap-2"
            aria-label="メインメニュー"
          >
            <Link
              to="/search"
              aria-label="検索"
              title="検索"
              className="rounded-full border border-white/15 px-3 py-2 text-xs font-black text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300 sm:px-4"
            >
              検索
            </Link>
            {isAuthenticated ? (
              <>
                <Link
                  to="/me/saved-videos"
                  aria-label="保存一覧"
                  title="保存一覧"
                  className="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-white/15 text-stone-200 transition hover:bg-white/[0.06] focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300 sm:flex sm:h-auto sm:w-auto sm:gap-2 sm:px-4 sm:py-2"
                >
                  <BookmarkIcon className="h-4 w-4" />
                  <span className="hidden text-xs font-black sm:inline">
                    保存一覧
                  </span>
                </Link>
                <Link
                  to="/me/videos"
                  className="rounded-full border border-white/15 px-3 py-2 text-xs font-black text-stone-200 transition hover:bg-white/[0.06] focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300 sm:px-4"
                >
                  自分の投稿
                </Link>
                {user?.role === "admin" && (
                  <Link
                    to="/admin/users"
                    aria-label="管理画面"
                    title="管理画面"
                    className="rounded-full border border-amber-300/40 px-2.5 py-2 text-xs font-black text-amber-200 transition hover:border-amber-300 hover:bg-amber-300/10 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300 sm:px-4"
                  >
                    <span className="sm:hidden">管理</span>
                    <span className="hidden sm:inline">管理画面</span>
                  </Link>
                )}
                <Link
                  to="/videos/upload"
                  className="rounded-full bg-amber-300 px-3 py-2 text-xs font-black text-stone-950 transition hover:bg-amber-200 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300 sm:px-4"
                >
                  投稿
                </Link>
                <button
                  type="button"
                  onClick={() => void handleLogout()}
                  disabled={isLoggingOut}
                  className="rounded-full border border-red-300/30 px-3 py-2 text-xs font-black text-red-100 transition hover:border-red-300/60 hover:bg-red-400/10 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-red-300 disabled:cursor-not-allowed disabled:opacity-60 sm:px-4"
                >
                  {isLoggingOut ? "ログアウト中" : "ログアウト"}
                </button>
              </>
            ) : (
              <>
                <Link
                  to="/login"
                  className="rounded-full border border-white/15 px-4 py-2 text-xs font-black text-stone-200 transition hover:bg-white/[0.06]"
                >
                  ログイン
                </Link>
                <Link
                  to="/signup"
                  className="rounded-full bg-amber-300 px-4 py-2 text-xs font-black text-stone-950 transition hover:bg-amber-200"
                >
                  新規登録
                </Link>
              </>
            )}
          </nav>
        </div>
      </header>

      {errorMessage !== "" && (
        <div
          className="fixed inset-x-4 top-20 z-40 mx-auto max-w-xl rounded-2xl border border-red-400/40 bg-red-950/95 px-4 py-3 text-sm text-red-100 shadow-2xl backdrop-blur"
          role="alert"
        >
          <p>{errorMessage}</p>
          {requestID !== "" && (
            <p className="mt-1 text-xs text-red-200/70">
              Request ID: {requestID}
            </p>
          )}
        </div>
      )}

      {isAuthLoading || isInitialLoading ? (
        <div
          className="flex min-h-[calc(100dvh-4.5rem)] items-center justify-center gap-3 text-sm font-bold text-stone-300"
          role="status"
        >
          <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
          リールを取得しています
        </div>
      ) : videos.length === 0 ? (
        <section className="grid min-h-[calc(100dvh-4.5rem)] place-items-center px-4 text-center">
          <div>
            <p className="text-5xl" aria-hidden="true">
              ☕
            </p>
            <h1 className="mt-5 text-3xl font-black text-white">
              公開中の動画はありません
            </h1>
          </div>
        </section>
      ) : (
        <section
          ref={reelListRef}
          className="h-[calc(100dvh-4.5rem)] snap-y snap-mandatory overflow-y-auto overscroll-y-contain scroll-smooth"
          aria-label="公開リール一覧"
        >
          {videos.map((video, index) => (
            <ReelVideo
              key={video.id}
              video={video}
              isActive={video.id === activeVideoID}
              shouldPreload={index === activeVideoIndex + 1}
              isAuthenticated={isAuthenticated}
              isSaving={savingVideoIDs.has(video.id)}
              onVisibilityChange={handleVisibilityChange}
              onToggleSaved={(targetVideo) =>
                void handleToggleSaved(targetVideo)
              }
              onLikeChange={handleLikeChange}
              onLikeNotFound={handleLikeNotFound}
              onLikeError={handleLikeError}
            />
          ))}

          {hasMore && (
            <div
              ref={loadMoreSentinelRef}
              className="h-px"
              aria-hidden="true"
            />
          )}

          {isLoadingMore && (
            <div
              className="flex h-20 items-center justify-center gap-3 text-sm font-bold text-stone-400"
              role="status"
            >
              <span className="h-4 w-4 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
              次のリールを取得しています
            </div>
          )}
        </section>
      )}
    </main>
  );
}
