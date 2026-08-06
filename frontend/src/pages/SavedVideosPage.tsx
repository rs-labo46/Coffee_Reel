import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { ApiClientError } from "../api/client";
import { listSavedVideos, removeSavedVideo } from "../api/video";
import { useAuth } from "../auth/useAuth";
import VideoCard from "../components/VideoCard";
import type { PublicVideo } from "../types/video";

const initialSavedVideoPromises = new Map<
  number,
  ReturnType<typeof listSavedVideos>
>();

// React StrictModeの再実行中にUser単位の初回保存一覧Requestを共有
function requestInitialSavedVideos(
  userID: number,
): ReturnType<typeof listSavedVideos> {
  const existingPromise = initialSavedVideoPromises.get(userID);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = listSavedVideos().finally(() => {
    initialSavedVideoPromises.delete(userID);
  });
  initialSavedVideoPromises.set(userID, request);

  return request;
}

// Cursor追加取得結果をID重複なしで結合
function appendSavedVideos(
  currentVideos: PublicVideo[],
  nextVideos: PublicVideo[],
): PublicVideo[] {
  const existingIDs = new Set(currentVideos.map((video) => video.id));

  return [
    ...currentVideos,
    ...nextVideos.filter((video) => !existingIDs.has(video.id)),
  ];
}

// API Errorを保存一覧画面用Messageへ変換
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

// 自分の保存動画、Cursor追加取得、保存解除を管理
export default function SavedVideosPage() {
  const { user } = useAuth();
  const loadMoreInFlightRef = useRef(false);
  const removingVideoIDsRef = useRef<Set<number>>(new Set());

  const [videos, setVideos] = useState<PublicVideo[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isInitialLoading, setIsInitialLoading] = useState<boolean>(true);
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false);
  const [removingVideoIDs, setRemovingVideoIDs] = useState<Set<number>>(
    new Set(),
  );
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");

  // 認証Userの保存一覧先頭Pageを取得
  useEffect(() => {
    if (user === null) {
      return;
    }

    let isActive = true;

    requestInitialSavedVideos(user.id)
      .then((response) => {
        if (!isActive) {
          return;
        }

        setVideos(response.items);
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        const errorView = errorViewOf(
          error,
          "保存した動画を取得できませんでした",
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
  }, [user]);

  // 次Cursorを使って保存一覧を追加取得
  async function handleLoadMore(): Promise<void> {
    if (!hasMore || nextCursor === null || loadMoreInFlightRef.current) {
      return;
    }

    loadMoreInFlightRef.current = true;
    setIsLoadingMore(true);
    setErrorMessage("");
    setRequestID("");

    try {
      const response = await listSavedVideos({
        cursor: nextCursor,
      });

      setVideos((currentVideos) =>
        appendSavedVideos(currentVideos, response.items),
      );
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more);
    } catch (error: unknown) {
      const errorView = errorViewOf(
        error,
        "次の保存動画を取得できませんでした",
      );
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      loadMoreInFlightRef.current = false;
      setIsLoadingMore(false);
    }
  }

  // 保存解除成功後に対象Videoを一覧から除外
  async function handleRemoveSaved(videoID: number): Promise<void> {
    if (removingVideoIDsRef.current.has(videoID)) {
      return;
    }

    removingVideoIDsRef.current.add(videoID);

    setRemovingVideoIDs((currentIDs) => {
      const nextIDs = new Set(currentIDs);
      nextIDs.add(videoID);
      return nextIDs;
    });
    setErrorMessage("");
    setRequestID("");

    try {
      await removeSavedVideo(videoID);

      setVideos((currentVideos) =>
        currentVideos.filter((video) => video.id !== videoID),
      );
    } catch (error: unknown) {
      const errorView = errorViewOf(error, "動画の保存を解除できませんでした");
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      removingVideoIDsRef.current.delete(videoID);

      setRemovingVideoIDs((currentIDs) => {
        const nextIDs = new Set(currentIDs);
        nextIDs.delete(videoID);
        return nextIDs;
      });
    }
  }

  return (
    <main className="min-h-dvh bg-[#100b08] px-4 py-6 text-stone-100 sm:px-6 lg:px-10">
      <div className="mx-auto w-full max-w-5xl">
        <header className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-xs font-black tracking-[0.2em] text-amber-300 uppercase">
              Saved videos
            </p>

            <h1 className="mt-2 text-3xl font-black text-white sm:text-4xl">
              保存した動画
            </h1>
          </div>

          <nav
            className="flex flex-wrap items-center gap-3"
            aria-label="保存一覧ナビゲーション"
          >
            <Link
              to="/"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              リール
            </Link>

            <Link
              to="/me/videos"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              自分の投稿
            </Link>
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

        {isInitialLoading ? (
          <div
            className="mt-10 flex items-center justify-center gap-3 py-16 text-sm font-bold text-stone-300"
            role="status"
          >
            <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
            保存した動画を読み込んでいます
          </div>
        ) : videos.length === 0 ? (
          <section className="mt-10 rounded-[2rem] border border-white/10 bg-white/[0.05] px-6 py-14 text-center">
            <p className="text-4xl" aria-hidden="true">
              ☕
            </p>

            <h2 className="mt-4 text-2xl font-black text-white">
              保存した動画はまだありません
            </h2>
            <Link
              to="/"
              className="mt-7 inline-flex min-h-12 items-center justify-center rounded-2xl bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200"
            >
              リールを見る
            </Link>
          </section>
        ) : (
          <section
            className="mt-8 grid gap-5 lg:grid-cols-2"
            aria-label="保存した動画一覧"
          >
            {videos.map((video) => {
              const isRemoving = removingVideoIDs.has(video.id);

              return (
                <VideoCard
                  key={video.id}
                  video={video}
                  to={`/videos/${video.id}`}
                  action={
                    <button
                      type="button"
                      onClick={() => void handleRemoveSaved(video.id)}
                      disabled={isRemoving}
                      className="inline-flex min-h-10 items-center justify-center rounded-full border border-red-300/30 px-4 py-2 text-xs font-black text-red-100 transition hover:bg-red-300/10 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {isRemoving ? "解除中" : "保存を解除"}
                    </button>
                  }
                />
              );
            })}
          </section>
        )}

        {!isInitialLoading && hasMore && nextCursor !== null && (
          <div className="mt-8 flex justify-center">
            <button
              type="button"
              onClick={() => void handleLoadMore()}
              disabled={isLoadingMore}
              className="inline-flex min-h-12 min-w-44 items-center justify-center rounded-2xl border border-amber-300/30 px-6 py-3 text-sm font-black text-amber-200 transition hover:bg-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {isLoadingMore ? "読み込み中" : "さらに読み込む"}
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
