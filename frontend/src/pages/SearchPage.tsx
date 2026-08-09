import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { Link, useSearchParams } from "react-router";

import { ApiClientError } from "../api/client";
import { listReels } from "../api/video";
import { useAuth } from "../auth/useAuth";
import LikeButton from "../components/LikeButton";
import VideoCard from "../components/VideoCard";
import type {
  CategoryCode,
  PublicSearchResultType,
  PublicVideo,
  VideoLikeState,
} from "../types/video";

const searchLimit = 20;
const maxTitleLength = 100;

const categories: ReadonlyArray<{
  value: CategoryCode;
  label: string;
}> = [
  { value: "brewing", label: "抽出" },
  { value: "roasting", label: "焙煎" },
  { value: "latte_art", label: "ラテアート" },
  { value: "beans", label: "コーヒー豆" },
  { value: "equipment", label: "器具" },
];

type SearchConditions = {
  title: string;
  category: CategoryCode | "";
};

type SearchPageContentProps = {
  queryTitle: string;
  queryCategory: CategoryCode | "";
  urlValidationError: string;
  isAuthenticated: boolean;
  isAuthLoading: boolean;
};

function countCharacters(value: string): number {
  return Array.from(value).length;
}

function categoryOf(value: string | null): CategoryCode | "" | null {
  if (value === null || value === "") {
    return "";
  }

  switch (value) {
    case "brewing":
    case "roasting":
    case "latte_art":
    case "beans":
    case "equipment":
      return value;
    default:
      return null;
  }
}

function conditionsFromURL(searchParams: URLSearchParams): {
  conditions: SearchConditions;
  error: string;
} {
  const title = (searchParams.get("title") ?? "").trim();
  const category = categoryOf(searchParams.get("category"));

  if (countCharacters(title) > maxTitleLength) {
    return {
      conditions: { title, category: category ?? "" },
      error: `タイトルは${maxTitleLength}文字以内で入力してください`,
    };
  }

  if (category === null) {
    return {
      conditions: { title, category: "" },
      error: "カテゴリーが正しくありません",
    };
  }

  return {
    conditions: { title, category },
    error: "",
  };
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

function errorViewOf(
  error: unknown,
  fallbackMessage: string,
): { message: string; requestID: string } {
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

function requireResultType(
  resultType: PublicSearchResultType | undefined,
): PublicSearchResultType {
  if (resultType === undefined) {
    throw new Error("GET /videos response is missing result_type");
  }

  return resultType;
}

function SearchPageContent({
  queryTitle,
  queryCategory,
  urlValidationError,
  isAuthenticated,
  isAuthLoading,
}: SearchPageContentProps) {
  const [, setSearchParams] = useSearchParams();
  const loadMoreInFlightRef = useRef(false);
  const loadMoreControllerRef = useRef<AbortController | null>(null);

  const [formTitle, setFormTitle] = useState<string>(queryTitle);
  const [formCategory, setFormCategory] = useState<CategoryCode | "">(
    queryCategory,
  );
  const [videos, setVideos] = useState<PublicVideo[]>([]);
  const [resultType, setResultType] = useState<PublicSearchResultType | null>(
    null,
  );
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isInitialLoading, setIsInitialLoading] = useState<boolean>(true);
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false);
  const [validationMessage, setValidationMessage] =
    useState<string>(urlValidationError);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");

  // URL条件単位でComponentを作り直し、古い検索RequestはAbortする
  useEffect(() => {
    if (isAuthLoading || urlValidationError !== "") {
      return;
    }

    const controller = new AbortController();
    let isActive = true;

    listReels(
      {
        title: queryTitle === "" ? undefined : queryTitle,
        category: queryCategory === "" ? undefined : queryCategory,
        limit: searchLimit,
      },
      controller.signal,
    )
      .then((response) => {
        if (!isActive) {
          return;
        }

        setVideos(response.items);
        setResultType(requireResultType(response.result_type));
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        const errorView = errorViewOf(error, "検索結果を取得できませんでした");
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
      controller.abort();
      loadMoreControllerRef.current?.abort();
    };
  }, [isAuthLoading, queryCategory, queryTitle, urlValidationError]);

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();

    const title = formTitle.trim();

    if (countCharacters(title) > maxTitleLength) {
      setValidationMessage(
        `タイトルは${maxTitleLength}文字以内で入力してください`,
      );
      return;
    }

    setValidationMessage("");

    const nextParams = new URLSearchParams();

    if (title !== "") {
      nextParams.set("title", title);
    }

    if (formCategory !== "") {
      nextParams.set("category", formCategory);
    }

    setSearchParams(nextParams);
  }

  function handleReset(): void {
    setFormTitle("");
    setFormCategory("");
    setValidationMessage("");
    setSearchParams(new URLSearchParams());
  }

  // 現在の検索条件を維持したままBackend生成Cursorを変更せず送る
  const handleLoadMore = useCallback(async (): Promise<void> => {
    if (!hasMore || nextCursor === null || loadMoreInFlightRef.current) {
      return;
    }

    const controller = new AbortController();
    loadMoreControllerRef.current?.abort();
    loadMoreControllerRef.current = controller;
    loadMoreInFlightRef.current = true;
    setIsLoadingMore(true);
    setErrorMessage("");
    setRequestID("");

    try {
      const response = await listReels(
        {
          title: queryTitle === "" ? undefined : queryTitle,
          category: queryCategory === "" ? undefined : queryCategory,
          limit: searchLimit,
          cursor: nextCursor,
        },
        controller.signal,
      );

      setVideos((currentVideos) => appendVideos(currentVideos, response.items));
      setResultType(requireResultType(response.result_type));
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more);
    } catch (error: unknown) {
      if (controller.signal.aborted) {
        return;
      }

      const errorView = errorViewOf(
        error,
        "次の検索結果を取得できませんでした",
      );
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      if (!controller.signal.aborted) {
        loadMoreInFlightRef.current = false;
        loadMoreControllerRef.current = null;
        setIsLoadingMore(false);
      }
    }
  }, [hasMore, nextCursor, queryCategory, queryTitle]);

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

  const hasConditions = queryTitle !== "" || queryCategory !== "";
  const showInitialLoading =
    isAuthLoading || (urlValidationError === "" && isInitialLoading);

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
            className="flex flex-wrap items-center justify-end gap-2"
            aria-label="検索画面ナビゲーション"
          >
            <Link
              to="/"
              className="rounded-full border border-white/15 px-4 py-2 text-xs font-black text-stone-200 transition hover:border-amber-300/50 hover:text-amber-200 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300"
            >
              リール
            </Link>
            {isAuthenticated ? (
              <Link
                to="/me/saved-videos"
                className="rounded-full border border-white/15 px-4 py-2 text-xs font-black text-stone-200 transition hover:bg-white/[0.06] focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300"
              >
                保存一覧
              </Link>
            ) : (
              <Link
                to="/login"
                className="rounded-full bg-amber-300 px-4 py-2 text-xs font-black text-stone-950 transition hover:bg-amber-200 focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-amber-300"
              >
                ログイン
              </Link>
            )}
          </nav>
        </div>
      </header>

      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 lg:px-10">
        <section className="border-b border-white/10 pb-6">
          <p className="text-xs font-black tracking-[0.22em] text-amber-300 uppercase">
            Search
          </p>
          <h1 className="mt-2 text-3xl font-black text-white sm:text-5xl">
            動画を検索
          </h1>
        </section>

        <section className="mt-7 rounded-[2rem] border border-white/10 bg-white/[0.05] p-5 shadow-xl shadow-black/15 sm:p-7">
          <form
            className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_240px_auto] lg:items-end"
            onSubmit={handleSubmit}
            noValidate
          >
            <div>
              <label
                htmlFor="search-title"
                className="text-sm font-black text-stone-200"
              >
                タイトル
              </label>
              <input
                id="search-title"
                type="search"
                value={formTitle}
                onChange={(event) => setFormTitle(event.target.value)}
                placeholder="例: ハンドドリップ"
                className="mt-2 min-h-12 w-full rounded-2xl border border-white/15 bg-black/20 px-4 py-3 text-sm text-white outline-none transition placeholder:text-stone-500 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10"
              />
              <p className="mt-2 text-xs font-bold text-stone-500">
                {countCharacters(formTitle.trim())}/{maxTitleLength}
              </p>
            </div>

            <div>
              <label
                htmlFor="search-category"
                className="text-sm font-black text-stone-200"
              >
                カテゴリー
              </label>
              <select
                id="search-category"
                value={formCategory}
                onChange={(event) =>
                  setFormCategory(event.target.value as CategoryCode | "")
                }
                className="mt-2 min-h-12 w-full rounded-2xl border border-white/15 bg-[#17100c] px-4 py-3 text-sm text-white outline-none transition focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10"
              >
                <option value="">すべて</option>
                {categories.map((category) => (
                  <option key={category.value} value={category.value}>
                    {category.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex flex-wrap gap-3">
              <button
                type="submit"
                className="min-h-12 rounded-2xl bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200"
              >
                検索
              </button>
              <button
                type="button"
                onClick={handleReset}
                className="min-h-12 rounded-2xl border border-white/15 px-5 py-3 text-sm font-black text-stone-200 transition hover:bg-white/[0.06]"
              >
                条件をクリア
              </button>
            </div>
          </form>

          {validationMessage !== "" && (
            <p
              className="mt-4 rounded-2xl border border-red-300/25 bg-red-400/10 px-4 py-3 text-sm font-bold text-red-100"
              role="alert"
            >
              {validationMessage}
            </p>
          )}
        </section>

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

        {resultType === "similar" && videos.length > 0 && (
          <div className="mt-6 rounded-2xl border border-amber-300/25 bg-amber-300/10 px-4 py-3 text-sm font-bold text-amber-100">
            一致する動画が見つからなかったため、近い動画を表示しています
          </div>
        )}

        <section className="mt-8" aria-label="検索結果">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <p className="text-xs font-black tracking-[0.18em] text-stone-500 uppercase">
                {hasConditions ? "Results" : "Public videos"}
              </p>
              <h2 className="mt-2 text-2xl font-black text-white">
                {hasConditions ? "検索結果" : "公開動画"}
              </h2>
            </div>
          </div>

          {showInitialLoading ? (
            <div
              className="mt-8 flex items-center justify-center gap-3 rounded-[2rem] border border-white/10 bg-white/[0.04] py-16 text-sm font-bold text-stone-300"
              role="status"
            >
              <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
              検索結果を取得しています
            </div>
          ) : videos.length === 0 ? (
            <div className="mt-8 rounded-[2rem] border border-white/10 bg-white/[0.04] px-6 py-14 text-center">
              <p className="text-4xl" aria-hidden="true">
                ☕
              </p>
              <h3 className="mt-4 text-2xl font-black text-white">
                {resultType === "similar"
                  ? "一致する動画も、近い動画も見つかりませんでした"
                  : "該当する動画はありません"}
              </h3>
            </div>
          ) : (
            <div className="mt-6 grid gap-5 lg:grid-cols-2">
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
            </div>
          )}
        </section>

        {!showInitialLoading && hasMore && nextCursor !== null && (
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

// URL Queryを正本にし、条件・認証Userが変わるたびに検索状態を作り直す
export default function SearchPage() {
  const [searchParams] = useSearchParams();
  const { isAuthenticated, isLoading: isAuthLoading, user } = useAuth();
  const parsed = conditionsFromURL(searchParams);
  const authKey =
    isAuthenticated && user !== null ? `user:${user.id}` : "guest";
  const queryKey = `${authKey}|${searchParams.toString()}`;

  return (
    <SearchPageContent
      key={queryKey}
      queryTitle={parsed.conditions.title}
      queryCategory={parsed.conditions.category}
      urlValidationError={parsed.error}
      isAuthenticated={isAuthenticated}
      isAuthLoading={isAuthLoading}
    />
  );
}
