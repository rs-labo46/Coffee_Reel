import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { Link, useParams } from "react-router";

import {
  getAdminVideo,
  hideAdminVideo,
  restoreAdminVideo,
} from "../api/admin_video";
import { ApiClientError } from "../api/client";
import VideoStatusBadge from "../components/VideoStatusBadge";
import type { CategoryCode } from "../types/video";
import type { AdminVideoDetailResponse } from "../types/admin_video";

const maxReasonLength = 500;

const initialDetailPromises = new Map<
  number,
  ReturnType<typeof getAdminVideo>
>();

function requestInitialDetail(
  videoID: number,
): ReturnType<typeof getAdminVideo> {
  const existingPromise = initialDetailPromises.get(videoID);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = getAdminVideo(videoID).finally(() => {
    initialDetailPromises.delete(videoID);
  });
  initialDetailPromises.set(videoID, request);

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

function parseVideoID(value: string | undefined): number | null {
  if (value === undefined || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }

  const videoID = Number(value);

  if (!Number.isSafeInteger(videoID) || videoID <= 0) {
    return null;
  }

  return videoID;
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

function canHide(detail: AdminVideoDetailResponse): boolean {
  return (
    detail.processing_status === "ready" &&
    detail.publish_status === "published"
  );
}

function canRestore(detail: AdminVideoDetailResponse): boolean {
  return (
    detail.processing_status === "ready" &&
    detail.publish_status === "hidden" &&
    detail.author.status === "active"
  );
}

export default function AdminVideoDetailPage() {
  const { video_id: videoIDParam } = useParams();
  const videoID = parseVideoID(videoIDParam);

  const [detail, setDetail] = useState<AdminVideoDetailResponse | null>(null);
  const [reason, setReason] = useState<string>("");
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [successMessage, setSuccessMessage] = useState<string>("");
  const [requestId, setRequestId] = useState<string>("");

  const isSubmittingRef = useRef(false);

  const loadDetail = useCallback(
    async (clearError = true): Promise<void> => {
      if (videoID === null) {
        return;
      }

      setIsLoading(true);

      if (clearError) {
        setErrorMessage("");
        setRequestId("");
      }

      try {
        const response = await getAdminVideo(videoID);
        setDetail(response);
      } catch (error: unknown) {
        if (error instanceof ApiClientError) {
          setErrorMessage(error.message);
          setRequestId(error.requestId);
        } else {
          setErrorMessage("投稿詳細の取得に失敗しました");
        }
      } finally {
        setIsLoading(false);
      }
    },
    [videoID],
  );

  useEffect(() => {
    if (videoID === null) {
      return;
    }

    let isActive = true;

    requestInitialDetail(videoID)
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
          setErrorMessage("投稿詳細の取得に失敗しました");
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
  }, [videoID]);

  async function handleStateChange(
    event: FormEvent<HTMLFormElement>,
  ): Promise<void> {
    event.preventDefault();

    if (
      detail === null ||
      videoID === null ||
      isSubmittingRef.current ||
      (!canHide(detail) && !canRestore(detail))
    ) {
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

    const action = detail.publish_status === "published" ? "hide" : "restore";

    isSubmittingRef.current = true;
    setIsSubmitting(true);

    try {
      if (action === "hide") {
        await hideAdminVideo(videoID, { reason: normalizedReason });
      } else {
        await restoreAdminVideo(videoID, { reason: normalizedReason });
      }

      setReason("");
      setSuccessMessage(
        action === "hide"
          ? "投稿を非公開にしました"
          : "投稿の公開を再開しました",
      );
      await loadDetail();
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
        setErrorMessage("投稿状態の変更に失敗しました");
      }
    } finally {
      isSubmittingRef.current = false;
      setIsSubmitting(false);
    }
  }

  if (videoID === null) {
    return (
      <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 text-stone-100">
        <div
          className="rounded-2xl border border-red-400/40 bg-red-950/40 px-5 py-4 text-sm text-red-100"
          role="alert"
        >
          投稿IDが正しくありません
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
          投稿詳細を取得しています
        </div>
      </main>
    );
  }

  const showAction =
    detail !== null &&
    detail.processing_status === "ready" &&
    (detail.publish_status === "published" ||
      detail.publish_status === "hidden");
  const restoreBlocked =
    detail !== null &&
    detail.publish_status === "hidden" &&
    detail.author.status === "suspended";

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-8 text-stone-100 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-5xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-7 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Coffee Reel Admin
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              投稿詳細
            </h1>
          </div>

          <nav className="flex flex-wrap gap-3" aria-label="管理者メニュー">
            <Link
              to="/admin/users"
              className="inline-flex min-h-11 items-center justify-center rounded-full border border-white/15 px-5 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              ユーザー管理
            </Link>
            <Link
              to="/admin/videos"
              className="inline-flex min-h-11 items-center justify-center rounded-full border border-amber-300/50 bg-amber-300/10 px-5 py-2 text-sm font-black text-amber-200"
            >
              投稿管理へ戻る
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
            <section className="mt-7 grid gap-6 rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(17rem,0.9fr)] lg:p-8">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-3">
                  <VideoStatusBadge
                    processingStatus={detail.processing_status}
                    publishStatus={detail.publish_status}
                  />
                  <span
                    className={`rounded-full px-3 py-1 text-xs font-black ${
                      detail.author.status === "active"
                        ? "bg-emerald-400/15 text-emerald-200"
                        : "bg-red-400/15 text-red-200"
                    }`}
                  >
                    投稿者:{" "}
                    {detail.author.status === "active"
                      ? "利用中"
                      : "利用停止中"}
                  </span>
                </div>

                <h2 className="mt-5 break-words text-2xl font-black text-white sm:text-4xl">
                  {detail.title}
                </h2>
                <p className="mt-4 break-words text-sm leading-8 text-stone-300">
                  {detail.description === ""
                    ? "説明はありません"
                    : detail.description}
                </p>

                <dl className="mt-7 grid gap-4 sm:grid-cols-2">
                  <div className="rounded-2xl border border-white/10 bg-black/20 p-5">
                    <dt className="text-xs font-black tracking-[0.16em] text-stone-500 uppercase">
                      Author
                    </dt>
                    <dd className="mt-2 break-words text-sm font-bold text-stone-100">
                      {detail.author.name}
                    </dd>
                  </div>
                  <div className="rounded-2xl border border-white/10 bg-black/20 p-5">
                    <dt className="text-xs font-black tracking-[0.16em] text-stone-500 uppercase">
                      Category
                    </dt>
                    <dd className="mt-2 text-sm font-bold text-stone-100">
                      {categoryLabel(detail.category)}
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
                  <div className="rounded-2xl border border-white/10 bg-black/20 p-5">
                    <dt className="text-xs font-black tracking-[0.16em] text-stone-500 uppercase">
                      Updated At
                    </dt>
                    <dd className="mt-2 text-sm font-bold text-stone-100">
                      {formatDate(detail.updated_at)}
                    </dd>
                  </div>
                </dl>
              </div>

              <div>
                {detail.playback_url !== null ? (
                  <video
                    controls
                    preload="metadata"
                    poster={detail.thumbnail_url ?? undefined}
                    className="aspect-[9/16] max-h-[70vh] w-full rounded-[1.75rem] bg-black object-contain"
                    aria-label="管理者確認用動画"
                  >
                    <source src={detail.playback_url} type="video/mp4" />
                  </video>
                ) : detail.thumbnail_url !== null ? (
                  <img
                    src={detail.thumbnail_url}
                    alt={`${detail.title}のサムネイル`}
                    className="aspect-[9/16] max-h-[70vh] w-full rounded-[1.75rem] bg-black object-contain"
                  />
                ) : (
                  <div className="grid aspect-[9/16] max-h-[70vh] place-items-center rounded-[1.75rem] border border-dashed border-white/15 bg-black/20 px-6 text-center text-sm font-bold text-stone-500">
                    現在の状態では管理者確認用動画を再生できません
                  </div>
                )}
              </div>
            </section>

            {showAction && (
              <section className="mt-6 rounded-[2rem] border border-white/10 bg-white/[0.05] p-6 sm:p-8">
                <p className="text-xs font-black tracking-[0.18em] text-stone-500 uppercase">
                  Management Action
                </p>
                <h2 className="mt-2 text-2xl font-black text-white">
                  {detail.publish_status === "published"
                    ? "投稿を非公開にする"
                    : "投稿の公開を再開する"}
                </h2>

                {restoreBlocked && (
                  <p className="mt-4 rounded-2xl border border-red-400/30 bg-red-950/30 px-4 py-3 text-sm text-red-100">
                    投稿者が利用停止中のため、公開を再開できません
                  </p>
                )}

                <form className="mt-6" onSubmit={handleStateChange}>
                  <label
                    htmlFor="admin-video-reason"
                    className="text-sm font-black text-stone-200"
                  >
                    {detail.publish_status === "published"
                      ? "非公開理由"
                      : "公開再開理由"}
                  </label>
                  <textarea
                    id="admin-video-reason"
                    value={reason}
                    onChange={(event) => setReason(event.target.value)}
                    disabled={isSubmitting || restoreBlocked}
                    rows={5}
                    className="mt-2 w-full resize-y rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-sm text-white outline-none transition placeholder:text-stone-600 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60"
                    placeholder="操作した理由を1〜500文字で入力してください"
                  />
                  <div className="mt-2 text-xs text-stone-500">
                    {countCharacters(reason.trim())}/{maxReasonLength}
                  </div>

                  <button
                    type="submit"
                    disabled={isSubmitting || restoreBlocked}
                    className={`mt-5 min-h-12 w-full rounded-full px-6 py-3 text-sm font-black transition disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto ${
                      detail.publish_status === "published"
                        ? "bg-red-300 text-red-950 hover:bg-red-200"
                        : "bg-emerald-300 text-emerald-950 hover:bg-emerald-200"
                    }`}
                  >
                    {isSubmitting
                      ? "処理中"
                      : detail.publish_status === "published"
                        ? "投稿を非公開にする"
                        : "投稿の公開を再開する"}
                  </button>
                </form>
              </section>
            )}
          </>
        )}
      </div>
    </main>
  );
}
