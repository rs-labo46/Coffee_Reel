import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";

import { ApiClientError } from "../api/client";
import {
  deleteVideo,
  getMyVideo,
  listMyVideos,
  republishVideo,
  setVideoPrivate,
} from "../api/video";
import { useAuth } from "../auth/useAuth";
import VideoCard from "../components/VideoCard";
import type {
  OwnedVideo,
  OwnedVideoDetail,
  VideoFailureCode,
  VideoProcessingStatus,
} from "../types/video";

const rawPollingIntervalMS = import.meta.env.VITE_VIDEO_POLLING_INTERVAL_MS;
const videoPollingIntervalMS = readPollingIntervalMS(rawPollingIntervalMS);

const initialMyVideosPromises = new Map<
  number,
  ReturnType<typeof listMyVideos>
>();

type UploadNavigationState = {
  uploadCompleted?: boolean;
  videoID?: number;
};

type PendingAction = {
  videoID: number;
  type: "private" | "publish" | "delete";
};

// Frontend設定から投稿状態Polling間隔を取得
function readPollingIntervalMS(value: string | undefined): number {
  if (value === undefined || !/^[1-9][0-9]*$/.test(value)) {
    throw new Error(
      "VITE_VIDEO_POLLING_INTERVAL_MS must be a positive integer",
    );
  }

  const intervalMS = Number(value);

  if (!Number.isSafeInteger(intervalMS) || intervalMS <= 0) {
    throw new Error(
      "VITE_VIDEO_POLLING_INTERVAL_MS must be a positive integer",
    );
  }

  return intervalMS;
}

// React StrictModeの再実行中に初回一覧Requestを共有
function requestInitialMyVideos(
  userID: number,
): ReturnType<typeof listMyVideos> {
  const existingPromise = initialMyVideosPromises.get(userID);

  if (existingPromise !== undefined) {
    return existingPromise;
  }

  const request = listMyVideos().finally(() => {
    initialMyVideosPromises.delete(userID);
  });
  initialMyVideosPromises.set(userID, request);

  return request;
}

// Router StateからUpload完了後のPolling対象IDを取得
function pollingVideoIDOf(state: unknown): number | null {
  if (typeof state !== "object" || state === null) {
    return null;
  }

  const navigationState = state as UploadNavigationState;
  const videoID = navigationState.videoID;

  if (
    navigationState.uploadCompleted !== true ||
    typeof videoID !== "number" ||
    !Number.isSafeInteger(videoID) ||
    videoID <= 0
  ) {
    return null;
  }

  return videoID;
}

// ProcessingStatusがPolling停止対象か判定
function isTerminalStatus(status: VideoProcessingStatus): boolean {
  return status === "ready" || status === "failed" || status === "expired";
}

// 投稿詳細Responseを一覧表示用Videoへ変換
function ownedVideoFromDetail(detail: OwnedVideoDetail): OwnedVideo {
  return {
    id: detail.id,
    title: detail.title,
    category: detail.category,
    processing_status: detail.processing_status,
    publish_status: detail.publish_status,
    thumbnail_url: detail.thumbnail_url,
    created_at: detail.created_at,
    updated_at: detail.updated_at,
  };
}

// 一覧へ同じIDを重複させず追加または更新
function upsertOwnedVideo(
  videos: OwnedVideo[],
  nextVideo: OwnedVideo,
): OwnedVideo[] {
  const index = videos.findIndex((video) => video.id === nextVideo.id);

  if (index === -1) {
    return [nextVideo, ...videos];
  }

  return videos.map((video) => (video.id === nextVideo.id ? nextVideo : video));
}

// Cursor追加取得結果をID重複なしで結合
function appendOwnedVideos(
  currentVideos: OwnedVideo[],
  nextVideos: OwnedVideo[],
): OwnedVideo[] {
  const existingIDs = new Set(currentVideos.map((video) => video.id));

  return [
    ...currentVideos,
    ...nextVideos.filter((video) => !existingIDs.has(video.id)),
  ];
}

// failure_codeをWorker内部情報を含まない安全なMessageへ変換
function failureMessageOf(code: VideoFailureCode | null): string {
  switch (code) {
    case "invalid_format":
      return "対応していない動画形式です";
    case "video_corrupt":
      return "動画を解析できませんでした";
    case "duration_exceeded":
      return "動画の再生時間が上限を超えています";
    case "size_exceeded":
      return "動画のファイル容量が上限を超えています";
    case "resolution_exceeded":
      return "動画の解像度が上限を超えています";
    case "invalid_aspect_ratio":
      return "動画の縦横比が9:16ではありません";
    case "frame_rate_exceeded":
      return "動画のフレームレートが上限を超えています";
    case "video_track_missing":
      return "動画に映像トラックがありません";
    case "processing_failed":
    case null:
      return "動画処理に失敗しました。時間を置いて再投稿してください";
  }
}

// API Errorを画面表示用情報へ変換
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

// 自分の投稿一覧、状態Polling、非公開、再公開、削除を管理
export default function MyVideosPage() {
  const location = useLocation();
  const { user } = useAuth();
  const pollingVideoID = pollingVideoIDOf(location.state);
  const pollingGenerationRef = useRef(0);
  const loadMoreInFlightRef = useRef(false);

  const [videos, setVideos] = useState<OwnedVideo[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState<boolean>(false);
  const [isInitialLoading, setIsInitialLoading] = useState<boolean>(true);
  const [isLoadingMore, setIsLoadingMore] = useState<boolean>(false);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(
    null,
  );
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");
  const [successMessage, setSuccessMessage] = useState<string>("");
  const [failureMessages, setFailureMessages] = useState<
    Record<number, string>
  >({});

  // 初回の自分の投稿一覧を取得
  useEffect(() => {
    if (user === null) {
      return;
    }

    let isActive = true;

    requestInitialMyVideos(user.id)
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
          "自分の投稿一覧を取得できませんでした",
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

  // Upload完了直後のVideoを終端状態まで制御された間隔で再取得
  useEffect(() => {
    if (pollingVideoID === null) {
      return;
    }

    const generation = pollingGenerationRef.current + 1;
    pollingGenerationRef.current = generation;

    let isStopped = false;
    let requestInFlight = false;
    let timerID: number | null = null;

    // 次回Pollingを設定間隔後に予約
    const scheduleNext = () => {
      if (isStopped || generation !== pollingGenerationRef.current) {
        return;
      }

      if (timerID !== null) {
        window.clearTimeout(timerID);
      }

      timerID = window.setTimeout(() => {
        void pollVideo();
      }, videoPollingIntervalMS);
    };

    // 投稿詳細を1回取得し、終端状態ならPollingを停止
    const pollVideo = async (): Promise<void> => {
      if (
        isStopped ||
        requestInFlight ||
        generation !== pollingGenerationRef.current
      ) {
        return;
      }

      if (document.visibilityState === "hidden") {
        scheduleNext();
        return;
      }

      requestInFlight = true;
      let shouldContinue: boolean;

      try {
        const detail = await getMyVideo(pollingVideoID);

        if (isStopped || generation !== pollingGenerationRef.current) {
          return;
        }

        setVideos((currentVideos) =>
          upsertOwnedVideo(currentVideos, ownedVideoFromDetail(detail)),
        );
        setErrorMessage("");
        setRequestID("");

        if (detail.processing_status === "failed") {
          setFailureMessages((currentMessages) => ({
            ...currentMessages,
            [detail.id]: failureMessageOf(detail.failure_code),
          }));
        }

        shouldContinue = !isTerminalStatus(detail.processing_status);
      } catch (error: unknown) {
        if (isStopped || generation !== pollingGenerationRef.current) {
          return;
        }

        if (error instanceof ApiClientError && error.status === 404) {
          setVideos((currentVideos) =>
            currentVideos.filter((video) => video.id !== pollingVideoID),
          );
          shouldContinue = false;
        } else {
          const errorView = errorViewOf(
            error,
            "投稿状態を取得できませんでした",
          );
          setErrorMessage(errorView.message);
          setRequestID(errorView.requestID);
          shouldContinue = true;
        }
      } finally {
        requestInFlight = false;
      }

      if (shouldContinue) {
        scheduleNext();
      }
    };

    // Tabへ戻った時に待機中Timerを解除してPollingを再開
    const handleVisibilityChange = () => {
      if (
        document.visibilityState !== "visible" ||
        isStopped ||
        generation !== pollingGenerationRef.current
      ) {
        return;
      }

      if (timerID !== null) {
        window.clearTimeout(timerID);
        timerID = null;
      }

      void pollVideo();
    };

    document.addEventListener("visibilitychange", handleVisibilityChange);
    void pollVideo();

    return () => {
      isStopped = true;
      document.removeEventListener("visibilitychange", handleVisibilityChange);

      if (timerID !== null) {
        window.clearTimeout(timerID);
      }
    };
  }, [pollingVideoID]);

  // Cursorを使って自分の投稿を追加取得
  async function handleLoadMore(): Promise<void> {
    if (!hasMore || nextCursor === null || loadMoreInFlightRef.current) {
      return;
    }

    loadMoreInFlightRef.current = true;
    setIsLoadingMore(true);
    setErrorMessage("");
    setRequestID("");

    try {
      const response = await listMyVideos({ cursor: nextCursor });
      setVideos((currentVideos) =>
        appendOwnedVideos(currentVideos, response.items),
      );
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more);
    } catch (error: unknown) {
      const errorView = errorViewOf(
        error,
        "自分の投稿を追加取得できませんでした",
      );
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    } finally {
      loadMoreInFlightRef.current = false;
      setIsLoadingMore(false);
    }
  }

  // 409競合後に投稿詳細を再取得して一覧状態を同期
  async function refreshVideo(videoID: number): Promise<void> {
    try {
      const detail = await getMyVideo(videoID);
      setVideos((currentVideos) =>
        upsertOwnedVideo(currentVideos, ownedVideoFromDetail(detail)),
      );

      if (detail.processing_status === "failed") {
        setFailureMessages((currentMessages) => ({
          ...currentMessages,
          [detail.id]: failureMessageOf(detail.failure_code),
        }));
      }
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 404) {
        setVideos((currentVideos) =>
          currentVideos.filter((video) => video.id !== videoID),
        );
        return;
      }

      throw error;
    }
  }

  // 409競合後の再取得失敗も含めて画面状態を更新
  async function syncVideoAfterConflict(videoID: number): Promise<void> {
    try {
      await refreshVideo(videoID);
      setErrorMessage("投稿状態が変更されていたため最新状態へ更新しました");
      setRequestID("");
    } catch (error: unknown) {
      const errorView = errorViewOf(
        error,
        "最新の投稿状態を取得できませんでした",
      );
      setErrorMessage(errorView.message);
      setRequestID(errorView.requestID);
    }
  }

  // published動画を投稿者操作でprivateへ変更
  async function handleSetPrivate(videoID: number): Promise<void> {
    if (pendingAction !== null) {
      return;
    }

    setPendingAction({ videoID, type: "private" });
    setErrorMessage("");
    setRequestID("");
    setSuccessMessage("");

    try {
      const response = await setVideoPrivate(videoID);
      setVideos((currentVideos) =>
        currentVideos.map((video) =>
          video.id === videoID
            ? {
                ...video,
                processing_status: response.processing_status,
                publish_status: response.publish_status,
              }
            : video,
        ),
      );
      setSuccessMessage("動画を非公開にしました");
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 409) {
        await syncVideoAfterConflict(videoID);
      } else {
        const errorView = errorViewOf(error, "動画を非公開にできませんでした");
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      }
    } finally {
      setPendingAction(null);
    }
  }

  // readyかつprivateの動画を投稿者操作でpublishedへ変更
  async function handleRepublish(videoID: number): Promise<void> {
    if (pendingAction !== null) {
      return;
    }

    setPendingAction({ videoID, type: "publish" });
    setErrorMessage("");
    setRequestID("");
    setSuccessMessage("");

    try {
      const response = await republishVideo(videoID);
      setVideos((currentVideos) =>
        currentVideos.map((video) =>
          video.id === videoID
            ? {
                ...video,
                processing_status: response.processing_status,
                publish_status: response.publish_status,
              }
            : video,
        ),
      );
      setSuccessMessage("動画を再公開しました");
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 409) {
        await syncVideoAfterConflict(videoID);
      } else {
        const errorView = errorViewOf(error, "動画を再公開できませんでした");
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      }
    } finally {
      setPendingAction(null);
    }
  }

  // 確認後に投稿を論理削除して一覧とPolling対象から除外
  async function handleDelete(videoID: number): Promise<void> {
    if (pendingAction !== null) {
      return;
    }

    const confirmed = window.confirm(
      "この動画を削除します。削除後は元に戻せません。",
    );

    if (!confirmed) {
      return;
    }

    setPendingAction({ videoID, type: "delete" });
    setErrorMessage("");
    setRequestID("");
    setSuccessMessage("");

    try {
      await deleteVideo(videoID);

      if (pollingVideoID === videoID) {
        pollingGenerationRef.current += 1;
      }

      setVideos((currentVideos) =>
        currentVideos.filter((video) => video.id !== videoID),
      );
      setFailureMessages((currentMessages) => {
        const nextMessages = { ...currentMessages };
        delete nextMessages[videoID];
        return nextMessages;
      });
      setSuccessMessage("動画を削除しました");
    } catch (error: unknown) {
      if (error instanceof ApiClientError && error.status === 404) {
        if (pollingVideoID === videoID) {
          pollingGenerationRef.current += 1;
        }

        setVideos((currentVideos) =>
          currentVideos.filter((video) => video.id !== videoID),
        );
        setFailureMessages((currentMessages) => {
          const nextMessages = { ...currentMessages };
          delete nextMessages[videoID];
          return nextMessages;
        });
        setSuccessMessage("削除済みの動画を一覧から除外しました");
      } else if (error instanceof ApiClientError && error.status === 409) {
        await syncVideoAfterConflict(videoID);
      } else {
        const errorView = errorViewOf(error, "動画を削除できませんでした");
        setErrorMessage(errorView.message);
        setRequestID(errorView.requestID);
      }
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-8 text-stone-100 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-6xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-7 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Coffee Reel
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              自分の投稿
            </h1>
          </div>

          <nav className="flex flex-wrap gap-3" aria-label="動画メニュー">
            <Link
              to="/"
              className="inline-flex min-h-11 items-center justify-center rounded-full border border-white/15 px-5 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
            >
              リールを見る
            </Link>
            <Link
              to="/videos/upload"
              className="inline-flex min-h-11 items-center justify-center rounded-full bg-amber-300 px-5 py-2 text-sm font-black text-stone-950 transition hover:bg-amber-200"
            >
              動画を投稿
            </Link>
          </nav>
        </header>

        {errorMessage !== "" && (
          <div
            className="mt-6 rounded-2xl border border-red-400/40 bg-red-950/40 px-4 py-3 text-sm text-red-100"
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

        {successMessage !== "" && (
          <div
            className="mt-6 rounded-2xl border border-emerald-400/40 bg-emerald-950/40 px-4 py-3 text-sm text-emerald-100"
            role="status"
          >
            {successMessage}
          </div>
        )}

        {isInitialLoading ? (
          <div
            className="flex min-h-80 items-center justify-center gap-3 text-sm font-bold text-stone-300"
            role="status"
          >
            <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
            投稿一覧を取得しています
          </div>
        ) : videos.length === 0 ? (
          <section className="mt-8 rounded-[2rem] border border-white/10 bg-white/[0.05] p-8 text-center sm:p-12">
            <p className="text-4xl" aria-hidden="true">
              ☕
            </p>
            <h2 className="mt-4 text-2xl font-black text-white">
              まだ投稿がありません
            </h2>
            <Link
              to="/videos/upload"
              className="mt-6 inline-flex min-h-11 items-center justify-center rounded-full bg-amber-300 px-6 py-2 text-sm font-black text-stone-950 transition hover:bg-amber-200"
            >
              動画を投稿
            </Link>
          </section>
        ) : (
          <section
            className="mt-8 grid gap-5 lg:grid-cols-2"
            aria-label="自分の投稿一覧"
          >
            {videos.map((video) => {
              const isActionPending = pendingAction?.videoID === video.id;
              const canSetPrivate =
                video.processing_status === "ready" &&
                video.publish_status === "published";
              const canRepublish =
                video.processing_status === "ready" &&
                video.publish_status === "private";
              const detailPath = canSetPrivate
                ? `/videos/${video.id}`
                : undefined;

              return (
                <VideoCard
                  key={video.id}
                  video={video}
                  to={detailPath}
                  action={
                    <div>
                      {video.processing_status === "failed" && (
                        <p className="mb-3 rounded-xl border border-red-300/20 bg-red-400/[0.08] px-3 py-2 text-xs font-bold leading-5 text-red-100">
                          {failureMessages[video.id] ??
                            "動画処理に失敗しました"}
                        </p>
                      )}

                      <div className="flex flex-wrap gap-2">
                        {canSetPrivate && (
                          <button
                            type="button"
                            onClick={() => void handleSetPrivate(video.id)}
                            disabled={pendingAction !== null}
                            className="inline-flex min-h-10 items-center justify-center rounded-full border border-sky-300/30 px-4 py-2 text-xs font-black text-sky-100 transition hover:bg-sky-300/10 disabled:cursor-not-allowed disabled:opacity-50"
                          >
                            {isActionPending &&
                            pendingAction?.type === "private"
                              ? "変更中"
                              : "非公開にする"}
                          </button>
                        )}

                        {canRepublish && (
                          <button
                            type="button"
                            onClick={() => void handleRepublish(video.id)}
                            disabled={pendingAction !== null}
                            className="inline-flex min-h-10 items-center justify-center rounded-full border border-emerald-300/30 px-4 py-2 text-xs font-black text-emerald-100 transition hover:bg-emerald-300/10 disabled:cursor-not-allowed disabled:opacity-50"
                          >
                            {isActionPending &&
                            pendingAction?.type === "publish"
                              ? "変更中"
                              : "再公開"}
                          </button>
                        )}

                        <button
                          type="button"
                          onClick={() => void handleDelete(video.id)}
                          disabled={pendingAction !== null}
                          className="inline-flex min-h-10 items-center justify-center rounded-full border border-red-300/30 px-4 py-2 text-xs font-black text-red-100 transition hover:bg-red-300/10 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          {isActionPending && pendingAction?.type === "delete"
                            ? "削除中"
                            : "削除"}
                        </button>
                      </div>
                    </div>
                  }
                />
              );
            })}
          </section>
        )}

        {videos.length > 0 && hasMore && (
          <div className="mt-8 flex justify-center">
            <button
              type="button"
              onClick={() => void handleLoadMore()}
              disabled={isLoadingMore || nextCursor === null}
              className="min-w-44 rounded-full bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {isLoadingMore ? "読み込み中" : "さらに読み込む"}
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
