import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router";

import {
  getAdminUser,
  resumeAdminUser,
  suspendAdminUser,
} from "../api/admin_user";
import { ApiClientError } from "../api/client";
import type { AdminUserDetailResponse } from "../types/admin_user";

const maxReasonLength = 500;

const initialDetailPromises = new Map<
  number,
  ReturnType<typeof getAdminUser>
>();

function requestInitialDetail(userID: number): ReturnType<typeof getAdminUser> {
  const existingPromise = initialDetailPromises.get(userID);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = getAdminUser(userID).finally(() => {
    initialDetailPromises.delete(userID);
  });
  initialDetailPromises.set(userID, request);

  return request;
}

function countCharacters(value: string): number {
  return Array.from(value).length;
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

function parseUserID(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }

  const userID = Number(value);

  if (!Number.isSafeInteger(userID) || userID <= 0) {
    return null;
  }

  return userID;
}

export default function AdminUserDetailPage() {
  const { user_id: userIDParam } = useParams();
  const userID = parseUserID(userIDParam);

  const [detail, setDetail] = useState<AdminUserDetailResponse | null>(null);
  const [reason, setReason] = useState<string>("");
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [successMessage, setSuccessMessage] = useState<string>("");
  const [requestId, setRequestId] = useState<string>("");

  const loadDetail = useCallback(
    async (clearError = true): Promise<void> => {
      if (userID === null) {
        return;
      }

      setIsLoading(true);

      if (clearError) {
        setErrorMessage("");
        setRequestId("");
      }

      try {
        const response = await getAdminUser(userID);
        setDetail(response);
      } catch (error: unknown) {
        if (error instanceof ApiClientError) {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        } else {
          setErrorMessage("ユーザー詳細の取得に失敗しました");
        }
      } finally {
        setIsLoading(false);
      }
    },
    [userID],
  );

  useEffect(() => {
    if (userID === null) {
      return;
    }

    let isActive = true;

    requestInitialDetail(userID)
      .then((response) => {
        if (isActive) {
          setDetail(response);
        }
      })
      .catch((error: unknown) => {
        if (!isActive) {
          return;
        }

        if (error instanceof ApiClientError) {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        } else {
          setErrorMessage("ユーザー詳細の取得に失敗しました");
        }
      })
      .finally(() => {
        if (isActive) {
          setIsLoading(false);
        }
      });

    return () => {
      isActive = false;
    };
  }, [userID]);

  async function handleStatusChange(
    event: FormEvent<HTMLFormElement>,
  ): Promise<void> {
    event.preventDefault();

    if (detail === null || userID === null || isSubmitting) {
      return;
    }

    const normalizedReason = reason.trim();
    const reasonLength = countCharacters(normalizedReason);

    setErrorMessage("");
    setSuccessMessage("");
    setRequestId("");

    if (reasonLength < 1 || reasonLength > maxReasonLength) {
      setErrorMessage("理由は1文字以上500文字以内で入力してください");
      return;
    }

    setIsSubmitting(true);

    try {
      const response =
        detail.status === "active"
          ? await suspendAdminUser(userID, { reason: normalizedReason })
          : await resumeAdminUser(userID, { reason: normalizedReason });

      setDetail((currentDetail) =>
        currentDetail === null
          ? null
          : {
              ...currentDetail,
              status: response.status,
            },
      );
      setReason("");
      setSuccessMessage(
        response.status === "suspended"
          ? "ユーザーを利用停止にしました"
          : "ユーザーの利用を再開しました",
      );

      if (response.status === "suspended") {
        await loadDetail();
      }
    } catch (error: unknown) {
      if (error instanceof ApiClientError) {
        if (error.status === 409) {
          setErrorMessage("既に状態が変更されています");
          await loadDetail(false);
        } else {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        }
      } else {
        setErrorMessage("ユーザー状態の変更に失敗しました");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  if (userID === null) {
    return (
      <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 text-stone-100">
        <div
          className="rounded-2xl border border-red-400/40 bg-red-950/40 px-5 py-4 text-sm text-red-100"
          role="alert"
        >
          ユーザーIDが正しくありません
        </div>
      </main>
    );
  }

  if (isLoading && detail === null) {
    return (
      <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 text-stone-100">
        <div
          className="flex items-center gap-3 text-sm font-bold text-stone-300"
          role="status"
        >
          <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
          ユーザー詳細を取得しています
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-8 text-stone-100 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-5xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-7 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Coffee Reel Admin
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              ユーザー詳細
            </h1>
          </div>

          <nav className="flex flex-wrap gap-3" aria-label="管理者メニュー">
            <Link
              to="/admin/users"
              className="inline-flex min-h-11 items-center justify-center rounded-full border border-amber-300/50 bg-amber-300/10 px-5 py-2 text-sm font-black text-amber-200"
            >
              一覧へ戻る
            </Link>
            <Link
              to="/admin/videos"
              className="inline-flex min-h-11 items-center justify-center rounded-full border border-white/15 px-5 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              投稿管理
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

        {successMessage !== "" && (
          <div
            className="mt-6 rounded-2xl border border-emerald-400/40 bg-emerald-950/40 px-4 py-3 text-sm text-emerald-100"
            role="status"
          >
            {successMessage}
          </div>
        )}

        {detail !== null && (
          <>
            <section className="mt-7 rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 sm:p-8">
              <div className="flex flex-wrap items-center gap-3">
                <h2 className="break-words text-2xl font-black text-white sm:text-4xl">
                  {detail.name}
                </h2>
                <span
                  className={`rounded-full px-3 py-1 text-xs font-black ${
                    detail.status === "active"
                      ? "bg-emerald-400/15 text-emerald-200"
                      : "bg-red-400/15 text-red-200"
                  }`}
                >
                  {detail.status === "active" ? "利用中" : "利用停止中"}
                </span>
              </div>

              <dl className="mt-7 grid gap-4 sm:grid-cols-2">
                <div className="rounded-2xl border border-white/10 bg-black/20 p-5">
                  <dt className="text-xs font-black tracking-[0.16em] text-stone-500 uppercase">
                    Email
                  </dt>
                  <dd className="mt-2 break-all text-sm font-bold text-stone-100">
                    {detail.email}
                  </dd>
                </div>
                <div className="rounded-2xl border border-white/10 bg-black/20 p-5">
                  <dt className="text-xs font-black tracking-[0.16em] text-stone-500 uppercase">
                    Created At
                  </dt>
                  <dd className="mt-2 text-sm font-bold text-stone-100">
                    {formatDate(detail.created_at)}
                  </dd>
                </div>
              </dl>
            </section>

            <section className="mt-6 rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 sm:p-8">
              <p className="text-xs font-black tracking-[0.18em] text-stone-500 uppercase">
                Management Action
              </p>
              <h2 className="mt-2 text-2xl font-black text-white">
                {detail.status === "active" ? "利用停止" : "利用再開"}
              </h2>

              <form className="mt-6" onSubmit={handleStatusChange}>
                <label
                  htmlFor="reason"
                  className="text-sm font-black text-stone-200"
                >
                  {detail.status === "active" ? "停止理由" : "再開理由"}
                </label>
                <textarea
                  id="reason"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  disabled={isSubmitting}
                  rows={5}
                  className="mt-2 w-full resize-y rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-sm text-white outline-none transition placeholder:text-stone-600 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60"
                  placeholder="操作した理由を1〜500文字で入力してください"
                />
                <div className="mt-2 flex items-center justify-between gap-4 text-xs text-stone-500">
                  <span>
                    {countCharacters(reason.trim())}/{maxReasonLength}
                  </span>
                </div>

                <button
                  type="submit"
                  disabled={isSubmitting}
                  className={`mt-5 min-h-12 w-full rounded-full px-6 py-3 text-sm font-black transition disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto ${
                    detail.status === "active"
                      ? "bg-red-300 text-red-950 hover:bg-red-200"
                      : "bg-emerald-300 text-emerald-950 hover:bg-emerald-200"
                  }`}
                >
                  {isSubmitting
                    ? "処理中"
                    : detail.status === "active"
                      ? "ユーザーを利用停止にする"
                      : "ユーザーの利用を再開する"}
                </button>
              </form>
            </section>

            <section className="mt-6 rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 sm:p-8">
              <div className="flex items-end justify-between gap-4">
                <div>
                  <p className="text-xs font-black tracking-[0.18em] text-stone-500 uppercase">
                    Videos
                  </p>
                  <h2 className="mt-2 text-2xl font-black text-white">
                    投稿動画
                  </h2>
                </div>
                <span className="text-sm font-black text-stone-400">
                  {detail.videos.length}件
                </span>
              </div>

              {detail.videos.length === 0 ? (
                <p className="mt-6 rounded-2xl border border-dashed border-white/10 px-5 py-10 text-center text-sm text-stone-500">
                  投稿動画はありません
                </p>
              ) : (
                <div className="mt-6 grid gap-4">
                  {detail.videos.map((video) => (
                    <Link
                      key={video.id}
                      to={`/admin/videos/${video.id}`}
                      aria-label={`${video.title}の投稿詳細を開く`}
                      className="group block rounded-2xl border border-white/10 bg-black/20 p-5 transition hover:border-amber-300/40 hover:bg-amber-300/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-300/70"
                    >
                      <h3 className="break-words text-base font-black text-white transition group-hover:text-amber-200">
                        {video.title}
                      </h3>
                      <div className="mt-3 flex flex-wrap gap-2 text-xs font-bold text-stone-400">
                        <span>{video.processing_status}</span>
                        <span aria-hidden="true">/</span>
                        <span>{video.publish_status}</span>
                        <span aria-hidden="true">/</span>
                        <span>{formatDate(video.created_at)}</span>
                      </div>
                    </Link>
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </div>
    </main>
  );
}
