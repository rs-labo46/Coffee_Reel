import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router";

import { ApiClientError } from "../api/client";
import { listAdminUsers } from "../api/admin_user";
import type { AdminUserListItem } from "../types/admin_user";

const pageLimit = 20;

let initialUsersPromise: ReturnType<typeof listAdminUsers> | null = null;

function requestInitialUsers(): ReturnType<typeof listAdminUsers> {
  if (initialUsersPromise === null) {
    initialUsersPromise = listAdminUsers(null, pageLimit).finally(() => {
      initialUsersPromise = null;
    });
  }

  return initialUsersPromise;
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

export default function AdminUsersPage() {
  const [items, setItems] = useState<AdminUserListItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestId, setRequestId] = useState<string>("");

  const isLoadingRef = useRef(true);
  const lastSuccessfulCursorRef = useRef<string | null | undefined>(undefined);

  const loadUsers = useCallback(
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
        const response = await listAdminUsers(cursor, pageLimit);

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
          setErrorMessage("ユーザー一覧の取得に失敗しました");
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

    requestInitialUsers()
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
          setErrorMessage("ユーザー一覧の取得に失敗しました");
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

  function handleReload(): void {
    void loadUsers(null, true);
  }

  function handleLoadMore(): void {
    if (!hasMore || nextCursor === null || isLoadingRef.current) {
      return;
    }

    void loadUsers(nextCursor, false);
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-8 text-stone-100 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-6xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-7 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Admin
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              ユーザー管理
            </h1>
          </div>

          <div className="flex flex-wrap gap-3">
            <Link
              to="/"
              className="rounded-full border border-white/15 px-4 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              トップへ戻る
            </Link>
            <button
              type="button"
              onClick={handleReload}
              disabled={isLoading}
              className="rounded-full bg-amber-300 px-4 py-2 text-sm font-black text-stone-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:opacity-60"
            >
              再読み込み
            </button>
          </div>
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
                ユーザー一覧を取得しています
              </div>
            </div>
          ) : items.length === 0 ? (
            <div className="rounded-[2rem] border border-white/10 bg-white/[0.04] px-6 py-16 text-center">
              <p className="text-lg font-black text-white">
                一般ユーザーは登録されていません
              </p>
            </div>
          ) : (
            <div className="grid gap-4">
              {items.map((item) => (
                <article
                  key={item.id}
                  className="grid gap-5 rounded-[2rem] border border-white/10 bg-white/[0.05] p-5 shadow-xl shadow-black/10 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:p-6"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-3">
                      <h2 className="break-words text-xl font-black text-white">
                        {item.name}
                      </h2>
                      <span
                        className={`rounded-full px-3 py-1 text-xs font-black ${
                          item.status === "active"
                            ? "bg-emerald-400/15 text-emerald-200"
                            : "bg-red-400/15 text-red-200"
                        }`}
                      >
                        {item.status === "active" ? "利用中" : "利用停止中"}
                      </span>
                    </div>

                    <p className="mt-3 break-all text-sm font-bold text-stone-300">
                      {item.email}
                    </p>
                    <p className="mt-2 text-xs text-stone-500">
                      登録日時: {formatDate(item.created_at)}
                    </p>
                  </div>

                  <Link
                    to={`/admin/users/${item.id}`}
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
