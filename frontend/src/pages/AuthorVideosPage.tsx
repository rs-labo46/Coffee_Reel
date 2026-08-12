import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { ApiClientError } from "../api/client";
import { listReels } from "../api/video";
import { useAuth } from "../auth/useAuth";
import LikeButton from "../components/LikeButton";
import VideoCard from "../components/VideoCard";
import type { PublicVideo, VideoLikeState } from "../types/video";

const initialAuthorVideoPromises = new Map<
  string,
  ReturnType<typeof listReels>
>();

function requestInitialAuthorVideos(
  requestKey: string,
  authorID: number,
): ReturnType<typeof listReels> {
  const existingPromise = initialAuthorVideoPromises.get(requestKey);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = listReels({ author_id: authorID }).finally(() => {
    initialAuthorVideoPromises.delete(requestKey);
  });
  initialAuthorVideoPromises.set(requestKey, request);

  return request;
}

function appendVideos(
  currentVideos: PublicVideo[],
  nextVideos: PublicVideo[],
): PublicVideo[] {
  const existingIDs = new Set(currentVideos.map((video) => video.id));

  return [
    ...currentVideos,
    ...nextVideos.filter((video) => !existingIDs.has(video.id)),
  ];
}

function parseAuthorID(rawAuthorID: string | undefined): number | null {
  if (rawAuthorID === undefined || !/^\d+$/.test(rawAuthorID)) {
    return null;
  }

  const authorID = Number(rawAuthorID);
  if (!Number.isSafeInteger(authorID) || authorID < 1) {
    return null;
  }

  return authorID;
}

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

type AuthorVideosPageContentProps = {
  authorID: number;
  isAuthenticated: boolean;
  requestKey: string;
};

function AuthorVideosPageContent({
  authorID,
  isAuthenticated,
  requestKey,
}: AuthorVideosPageContentProps) {
  const loadMoreInFlightRef = useRef(false);

  const [videos, setVideos] = useState<PublicVideo[]>([]);
  const [authorName, setAuthorName] = useState<string>("");
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isInitialLoading, setIsInitialLoading] = useState<boolean>(true);
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");

  useEffect(() => {
    let isActive = true;

    requestInitialAuthorVideos(requestKey, authorID)
      .then((response) => {
        if (!isActive) {
          return;
        }

        setVideos(response.items);
        setAuthorName(response.items[0]?.author.name ?? "");
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        const errorView = errorViewOf(
          error,
          "投稿者の公開動画を取得できませんでした",
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
  }, [authorID, requestKey]);

  async function handleLoadMore(): Promise<void> {
    if (!hasMore || nextCursor === null || loadMoreInFlightRef.current) {
      return;
    }

    loadMoreInFlightRef.current = true;
    setIsLoadingMore(true);
    setErrorMessage("");
    setRequestID("");

    try {
      const response = await listReels({
        author_id: authorID,
        cursor: nextCursor,
      });
      setVideos((currentVideos) =>
        appendVideos(currentVideos, response.items),
      );
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more);
    } catch (error: unknown) {
      const errorView = errorViewOf(
        error,
        "次の公開動画を取得できませんでした",
      );
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      loadMoreInFlightRef.current = false;
      setIsLoadingMore(false);
    }
  }

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
    setVideos((currentVideos) =>
      currentVideos.filter((video) => video.id !== videoID),
    );
  }

  function handleLikeError(error: unknown): void {
    const errorView = errorViewOf(error, "いいね状態を更新できませんでした");
    setErrorMessage(errorView.message);
    setRequestID(errorView.requestID);
  }

  return (
    <main className="min-h-dvh bg-[#100b08] px-4 py-6 text-stone-100 sm:px-6 lg:px-10">
      <div className="mx-auto w-full max-w-5xl">
        <header className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <p className="text-xs font-black tracking-[0.2em] text-amber-300 uppercase">
              Author videos
            </p>
            <h1 className="mt-2 text-3xl font-black text-white sm:text-4xl">
              {authorName === "" ? "投稿者の公開動画" : authorName}
            </h1>
          </div>

          <nav
            className="flex flex-wrap items-center gap-3"
            aria-label="投稿者動画一覧ナビゲーション"
          >
            <Link
              to="/"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              リール
            </Link>
            <Link
              to="/search"
              className="rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
            >
              検索
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
            投稿者の公開動画を読み込んでいます
          </div>
        ) : videos.length === 0 && errorMessage === "" ? (
          <section className="mt-10 rounded-[2rem] border border-white/10 bg-white/[0.05] px-6 py-14 text-center">
            <p className="text-4xl" aria-hidden="true">
              ☕
            </p>
            <h2 className="mt-4 text-2xl font-black text-white">
              公開動画はありません
            </h2>
          </section>
        ) : (
          <section
            className="mt-8 grid gap-5 lg:grid-cols-2"
            aria-label="投稿者の公開動画一覧"
          >
            {videos.map((video) => (
              <VideoCard
                key={video.id}
                video={video}
                to={`/videos/${video.id}`}
                action={
                  <LikeButton
                    videoID={video.id}
                    likeCount={video.like_count}
                    isLiked={video.is_liked}
                    isAuthenticated={isAuthenticated}
                    onChange={handleLikeChange}
                    onNotFound={() => handleLikeNotFound(video.id)}
                    onError={handleLikeError}
                  />
                }
              />
            ))}
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

function AuthorRouteError() {
  return (
    <main className="min-h-dvh bg-[#100b08] px-4 py-6 text-stone-100 sm:px-6 lg:px-10">
      <div className="mx-auto w-full max-w-5xl">
        <div
          className="rounded-2xl border border-red-300/25 bg-red-400/10 px-4 py-3 text-sm font-bold text-red-100"
          role="alert"
        >
          投稿者IDが正しくありません
        </div>
        <Link
          to="/"
          className="mt-6 inline-flex rounded-full border border-white/10 px-4 py-2 text-sm font-bold text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200"
        >
          リールへ戻る
        </Link>
      </div>
    </main>
  );
}

export default function AuthorVideosPage() {
  const { author_id: rawAuthorID } = useParams<{ author_id: string }>();
  const authorID = parseAuthorID(rawAuthorID);
  const { isAuthenticated, isLoading: isAuthLoading, user } = useAuth();

  if (isAuthLoading) {
    return (
      <main className="grid min-h-dvh place-items-center bg-[#100b08] text-stone-300">
        <p role="status" className="text-sm font-bold">
          読み込んでいます
        </p>
      </main>
    );
  }

  if (authorID === null) {
    return <AuthorRouteError />;
  }

  const authKey = isAuthenticated && user !== null ? `user:${user.id}` : "guest";
  const requestKey = `${authKey}|author:${authorID}`;

  return (
    <AuthorVideosPageContent
      key={requestKey}
      authorID={authorID}
      isAuthenticated={isAuthenticated}
      requestKey={requestKey}
    />
  );
}
