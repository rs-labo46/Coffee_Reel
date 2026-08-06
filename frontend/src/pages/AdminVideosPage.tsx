import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { listAdminVideos } from "../api/admin_video";
import { ApiClientError } from "../api/client";
import VideoStatusBadge from "../components/VideoStatusBadge";

import type { CategoryCode } from "../types/video";
import type { AdminVideoListItem } from "../types/admin_video";

const pageLimit = 20;

let initialVideosPromise: ReturnType<typeof listAdminVideos> | null = null;

function requestInitialVideos(): ReturnType<typeof listAdminVideos> {
  if (initialVideosPromise === null) {
    initialVideosPromise = listAdminVideos(null, pageLimit).finally(() => {
      initialVideosPromise = null;
    });
  }

  return initialVideosPromise;
}

function formatDate(value: string): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("ja-JP", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function categoryLabel(category: CategoryCode): string {
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

export default function AdminVideosPage() {
  const [items, setItems] = useState<AdminVideoListItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestId, setRequestId] = useState<string>("");

  const isLoadingRef = useRef(true);
  const lastSuccessfulCursorRef = useRef<string | null | undefined>(undefined);

  const loadVideos = useCallback(
    async (cursor: string | null, replace: boolean): Promise<void> => {
      if (isLoadingRef.current) {
        return;
      }

      if (!replace && lastSuccessfulCursorRef.current === cursor) {
        return;
      }

      isLoadingRef.current = true;
      setIsLoading(true);
      setErrorMessage("");
      setRequestId("");

      if (replace) {
        setItems([]);
        setNextCursor(null);
        setHasMore(false);
        lastSuccessfulCursorRef.current = undefined;
      }

      try {
        const response = await listAdminVideos(cursor, pageLimit);

        setItems((currentItems) =>
          replace ? response.items : [...currentItems, ...response.items],
        );
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
        lastSuccessfulCursorRef.current = cursor;
      } catch (error: unknown) {
        if (error instanceof ApiClientError) {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        } else {
          setErrorMessage("投稿一覧の取得に失敗しました");
        }
      } finally {
        isLoadingRef.current = false;
        setIsLoading(false);
      }
    },
    [],
  );

  useEffect(() => {
    let isActive = true;

    requestInitialVideos()
      .then((response) => {
        if (!isActive) {
          return;
        }

        setItems(response.items);
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
        lastSuccessfulCursorRef.current = null;
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        if (error instanceof ApiClientError) {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        } else {
          setErrorMessage("投稿一覧の取得に失敗しました");
        }
      })
      .finally(() => {
        if (!isActive) {
          return;
        }

        isLoadingRef.current = false;
        setIsLoading(false);
      });

    return () => {
      isActive = false;
    };
  }, []);

  function handleLoadMore(): void {
    if (!hasMore || nextCursor === null || isLoadingRef.current) {
      return;
    }

    void loadVideos(nextCursor, false);
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-8 text-stone-100 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-6xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-7 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Coffee Reel Admin
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              投稿管理
            </h1>
          </div>

          <nav className="flex flex-wrap gap-3" aria-label="管理者メニュー">
            <Link
              to="/admin/users"
              className="rounded-full border border-white/15 px-4 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              ユーザー管理
            </Link>
            <Link
              to="/admin/videos"
              aria-current="page"
              className="rounded-full border border-amber-300/50 bg-amber-300/10 px-4 py-2 text-sm font-black text-amber-200"
            >
              投稿管理
            </Link>
            <Link
              to="/"
              className="rounded-full border border-white/15 px-4 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              トップへ戻る
            </Link>
          </nav>
        </header>

        {errorMessage !== "" && (
          <div
            className="mt-6 rounded-2xl border border-red-400/40 bg-red-950/40 px-4 py-3 text-sm text-red-100"
            role="alert"
          >
            <p>{errorMessage}</p>
            {requestId !== "" && (
              <p className="mt-1 text-xs text-red-200/70">
                Request ID: {requestId}
              </p>
            )}
          </div>
        )}

        <section className="mt-7">
          {items.length === 0 && isLoading ? (
            <div
              className="grid min-h-64 place-items-center rounded-[2rem] border border-white/10 bg-white/[0.04]"
              role="status"
            >
              <div className="flex items-center gap-3 text-sm font-bold text-stone-300">
                <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
                投稿一覧を取得しています
              </div>
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-[2rem] border border-white/10 bg-white/[0.04] px-6 py-16 text-center">
              <p className="text-lg font-black text-white">
                管理対象の投稿はありません
              </p>
            </div>
          ) : (
            <div className="grid gap-4">
              {items.map((item) => (
                <article
                  key={item.id}
                  className="grid gap-5 rounded-[2rem] border border-white/10 bg-white/[0.05] p-5 shadow-xl shadow-black/10 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center lg:p-6"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-3">
                      <VideoStatusBadge
                        processingStatus={item.processing_status}
                        publishStatus={item.publish_status}
                      />
                      <span
                        className={`rounded-full px-3 py-1 text-xs font-black ${
                          item.author.status === "active"
                            ? "bg-emerald-400/15 text-emerald-200"
                            : "bg-red-400/15 text-red-200"
                        }`}
                      >
                        投稿者:{" "}
                        {item.author.status === "active"
                          ? "利用中"
                          : "利用停止中"}
                      </span>
                    </div>

                    <h2 className="mt-4 break-words text-xl font-black text-white sm:text-2xl">
                      {item.title}
                    </h2>
                    <p className="mt-2 break-words text-sm leading-7 text-stone-300">
                      {item.description === ""
                        ? "説明はありません"
                        : item.description}
                    </p>

                    <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-xs font-bold text-stone-500">
                      <span>投稿者: {item.author.name}</span>
                      <span>カテゴリー: {categoryLabel(item.category)}</span>
                      <span>投稿日時: {formatDate(item.created_at)}</span>
                    </div>
                  </div>

                  <Link
                    to={`/admin/videos/${item.id}`}
                    className="inline-flex min-h-11 items-center justify-center rounded-full border border-amber-300/40 px-5 py-2 text-sm font-black text-amber-200 transition hover:border-amber-300 hover:bg-amber-300/10"
                  >
                    詳細を確認
                  </Link>
                </article>
              ))}
            </div>
          )}
        </section>

        {items.length > 0 && hasMore && (
          <div className="mt-8 flex justify-center">
            <button
              type="button"
              onClick={handleLoadMore}
              disabled={isLoading || nextCursor === null}
              className="min-w-44 rounded-full bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {isLoading ? "読み込み中" : "さらに読み込む"}
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
