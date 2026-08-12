import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from "react";
import { Link, useNavigate } from "react-router";

import { ApiClientError } from "../api/client";
import { uploadVideo, VideoUploadError } from "../api/upload";
import { completeVideoUpload, startVideoUpload } from "../api/video";
import UploadProgress from "../components/UploadProgress";
import type { CategoryCode, VideoFileContentType } from "../types/video";

const maxTitleLength = 100;
const maxDescriptionLength = 1000;
const maxFileSizeBytes = 50_000_000;
const maxDurationSeconds = 10;
const maxVideoWidth = 1080;
const maxVideoHeight = 1920;

const inputClass =
  "mt-2 w-full rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-3.5 text-[15px] text-stone-200 outline-none transition placeholder:text-stone-500 hover:border-white/20 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60";

const categoryOptions: ReadonlyArray<{
  value: CategoryCode;
  label: string;
}> = [
  { value: "brewing", label: "抽出" },
  { value: "roasting", label: "焙煎" },
  { value: "latte_art", label: "ラテアート" },
  { value: "beans", label: "コーヒー豆" },
  { value: "equipment", label: "器具" },
];

type VideoMetadata = {
  durationSeconds: number;
  width: number;
  height: number;
};

type SelectedVideo = {
  file: File;
  contentType: VideoFileContentType;
  metadata: VideoMetadata;
};

type IdempotencyState = {
  fingerprint: string;
  key: string;
};

type SubmissionPhase = "idle" | "uploading" | "completing";

// Unicode文字数をBackendと同じ単位で計測
function countCharacters(value: string): number {
  return Array.from(value).length;
}

// File名の拡張子とBrowserのMIME Typeから送信用Content-Typeを判定
function resolveContentType(file: File): VideoFileContentType | null {
  const fileName = file.name.toLowerCase();
  const mimeType = file.type.toLowerCase().trim();

  if (fileName.endsWith(".mp4")) {
    return mimeType === "" || mimeType === "video/mp4" ? "video/mp4" : null;
  }

  if (fileName.endsWith(".mov")) {
    return mimeType === "" || mimeType === "video/quicktime"
      ? "video/quicktime"
      : null;
  }

  return null;
}

// Object URLから動画の再生時間と表示サイズを取得
function readVideoMetadata(objectURL: string): Promise<VideoMetadata> {
  return new Promise<VideoMetadata>((resolve, reject) => {
    const video = document.createElement("video");

    // Metadata取得後のEventとMedia参照を解除
    const cleanup = () => {
      video.onloadedmetadata = null;
      video.onerror = null;
      video.removeAttribute("src");
      video.load();
    };

    video.preload = "metadata";
    video.muted = true;
    video.playsInline = true;

    video.onloadedmetadata = () => {
      const metadata: VideoMetadata = {
        durationSeconds: video.duration,
        width: video.videoWidth,
        height: video.videoHeight,
      };

      cleanup();
      resolve(metadata);
    };

    video.onerror = () => {
      cleanup();
      reject(new Error("video metadata could not be loaded"));
    };

    video.src = objectURL;
  });
}

// 選択動画が容量、形式、時間、解像度、縦横比の条件内か確認
function validateSelectedVideo(
  file: File,
  contentType: VideoFileContentType | null,
  metadata: VideoMetadata | null,
): string {
  if (file.size < 1) {
    return "空の動画は選択できません";
  }

  if (file.size > maxFileSizeBytes) {
    return "動画の容量は50MB以下にしてください";
  }

  if (contentType === null) {
    return "MP4またはMOV形式の動画を選択してください";
  }

  if (metadata === null) {
    return "動画情報を読み取れませんでした";
  }

  if (
    !Number.isFinite(metadata.durationSeconds) ||
    metadata.durationSeconds <= 0
  ) {
    return "動画の再生時間を確認できませんでした";
  }

  if (metadata.durationSeconds > maxDurationSeconds) {
    return "動画の再生時間は10秒以下にしてください";
  }

  if (metadata.width > maxVideoWidth || metadata.height > maxVideoHeight) {
    return "動画の解像度は1080×1920以下にしてください";
  }

  if (
    metadata.width < 1 ||
    metadata.height < 1 ||
    metadata.width * 16 !== metadata.height * 9
  ) {
    return "縦横比が9:16の縦動画を選択してください";
  }

  return "";
}

// 投稿フォームの入力値をBackend条件へ合わせて確認
function validateForm(
  title: string,
  description: string,
  selectedVideo: SelectedVideo | null,
): string {
  const normalizedTitle = title.trim();
  const titleLength = countCharacters(normalizedTitle);

  if (titleLength < 1 || titleLength > maxTitleLength) {
    return "タイトルは1文字以上100文字以内で入力してください";
  }

  if (countCharacters(description) > maxDescriptionLength) {
    return "説明は1000文字以内で入力してください";
  }

  if (selectedVideo === null) {
    return "投稿する動画を選択してください";
  }

  return "";
}

// 現在の投稿内容からIdempotency-Key再利用判定用Fingerprintを生成
function createSubmissionFingerprint(
  title: string,
  description: string,
  category: CategoryCode,
  selectedVideo: SelectedVideo,
): string {
  const { file } = selectedVideo;

  return JSON.stringify({
    title: title.trim(),
    description,
    category,
    fileName: file.name,
    fileSize: file.size,
    fileLastModified: file.lastModified,
    contentType: selectedVideo.contentType,
  });
}

// 暗号学的乱数からIdempotency-Keyを生成
function createIdempotencyKey(): string {
  if (typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }

  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);

  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

// 同じ投稿内容の再送で使うIdempotency-Keyを取得
function getIdempotencyKey(
  state: IdempotencyState | null,
  fingerprint: string,
): IdempotencyState {
  if (state !== null && state.fingerprint === fingerprint) {
    return state;
  }

  return {
    fingerprint,
    key: createIdempotencyKey(),
  };
}

// API・Storage Errorを利用者向けMessageへ変換
function errorMessageOf(error: unknown): {
  message: string;
  requestID: string;
} {
  if (error instanceof ApiClientError) {
    return {
      message: error.message,
      requestID: error.requestId,
    };
  }

  if (error instanceof VideoUploadError) {
    return {
      message: error.message,
      requestID: "",
    };
  }

  return {
    message: "動画の投稿に失敗しました",
    requestID: "",
  };
}

// 動画選択、Preview、投稿開始、Storage Upload、完了通知を管理
export default function VideoUploadPage() {
  const navigate = useNavigate();
  const abortControllerRef = useRef<AbortController | null>(null);
  const fileSelectionRevisionRef = useRef(0);
  const idempotencyStateRef = useRef<IdempotencyState | null>(null);

  const [title, setTitle] = useState<string>("");
  const [description, setDescription] = useState<string>("");
  const [category, setCategory] = useState<CategoryCode>("brewing");
  const [selectedVideo, setSelectedVideo] = useState<SelectedVideo | null>(
    null,
  );
  const [previewURL, setPreviewURL] = useState<string>("");
  const [progress, setProgress] = useState({
    loadedBytes: 0,
    totalBytes: 0,
    percentage: 0,
  });
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [requestID, setRequestID] = useState<string>("");
  const [submissionPhase, setSubmissionPhase] =
    useState<SubmissionPhase>("idle");
  const [pendingCompletionVideoID, setPendingCompletionVideoID] = useState<
    number | null
  >(null);
  const [isCancelling, setIsCancelling] = useState<boolean>(false);

  const isSubmitting = submissionPhase !== "idle";
  const isUploading = submissionPhase === "uploading";
  const isFormLocked = isSubmitting || pendingCompletionVideoID !== null;

  // Preview URL変更時に前のObject URLを破棄
  useEffect(() => {
    return () => {
      if (previewURL !== "") {
        URL.revokeObjectURL(previewURL);
      }
    };
  }, [previewURL]);

  // Page破棄時に実行中のStorage Uploadを中断
  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
    };
  }, []);

  // File選択時にClient条件を確認しPreviewを反映
  async function handleFileChange(
    event: ChangeEvent<HTMLInputElement>,
  ): Promise<void> {
    const file = event.target.files?.[0] ?? null;
    const selectionRevision = fileSelectionRevisionRef.current + 1;
    fileSelectionRevisionRef.current = selectionRevision;

    setErrorMessage("");
    setRequestID("");
    setSelectedVideo(null);
    setPreviewURL("");
    setProgress({ loadedBytes: 0, totalBytes: 0, percentage: 0 });

    if (file === null) {
      return;
    }

    const contentType = resolveContentType(file);

    if (file.size < 1) {
      setErrorMessage("空の動画は選択できません");
      event.target.value = "";
      return;
    }

    if (file.size > maxFileSizeBytes) {
      setErrorMessage("動画の容量は50MB以下にしてください");
      event.target.value = "";
      return;
    }

    if (contentType === null) {
      setErrorMessage("MP4またはMOV形式の動画を選択してください");
      event.target.value = "";
      return;
    }

    const objectURL = URL.createObjectURL(file);

    try {
      const metadata = await readVideoMetadata(objectURL);

      if (fileSelectionRevisionRef.current !== selectionRevision) {
        URL.revokeObjectURL(objectURL);
        return;
      }

      const validationMessage = validateSelectedVideo(
        file,
        contentType,
        metadata,
      );

      if (validationMessage !== "" || contentType === null) {
        URL.revokeObjectURL(objectURL);
        setErrorMessage(validationMessage);
        event.target.value = "";
        return;
      }

      setSelectedVideo({ file, contentType, metadata });
      setPreviewURL(objectURL);
      setProgress({
        loadedBytes: 0,
        totalBytes: file.size,
        percentage: 0,
      });
    } catch {
      URL.revokeObjectURL(objectURL);
      setErrorMessage("動画情報を読み取れませんでした");
      event.target.value = "";
    }
  }

  // Upload完了通知を送信し自分の投稿画面へ移動
  async function notifyUploadComplete(videoID: number): Promise<void> {
    setSubmissionPhase("completing");

    await completeVideoUpload(videoID);
    setPendingCompletionVideoID(null);

    navigate("/me/videos", {
      replace: true,
      state: {
        uploadCompleted: true,
        videoID,
      },
    });
  }

  // 投稿開始、Storage直接Upload、完了通知を順番に実行
  async function handleSubmit(
    event: FormEvent<HTMLFormElement>,
  ): Promise<void> {
    event.preventDefault();

    if (isSubmitting) {
      return;
    }

    setErrorMessage("");
    setRequestID("");
    setIsCancelling(false);

    if (pendingCompletionVideoID !== null) {
      try {
        await notifyUploadComplete(pendingCompletionVideoID);
      } catch (error: unknown) {
        const result = errorMessageOf(error);
        setErrorMessage(result.message);
        setRequestID(result.requestID);
      } finally {
        setSubmissionPhase("idle");
      }

      return;
    }

    const validationMessage = validateForm(title, description, selectedVideo);

    if (validationMessage !== "" || selectedVideo === null) {
      setErrorMessage(validationMessage);
      return;
    }

    const fingerprint = createSubmissionFingerprint(
      title,
      description,
      category,
      selectedVideo,
    );
    const idempotencyState = getIdempotencyKey(
      idempotencyStateRef.current,
      fingerprint,
    );
    idempotencyStateRef.current = idempotencyState;

    const abortController = new AbortController();
    abortControllerRef.current = abortController;
    setSubmissionPhase("uploading");
    setProgress({
      loadedBytes: 0,
      totalBytes: selectedVideo.file.size,
      percentage: 0,
    });

    try {
      const result = await startVideoUpload(
        {
          title: title.trim(),
          description,
          category,
          file_content_type: selectedVideo.contentType,
          file_size_bytes: selectedVideo.file.size,
        },
        idempotencyState.key,
      );

      await uploadVideo({
        file: selectedVideo.file,
        target: result.upload,
        signal: abortController.signal,
        onProgress: setProgress,
      });

      setPendingCompletionVideoID(result.video.id);
      abortControllerRef.current = null;
      await notifyUploadComplete(result.video.id);
    } catch (error: unknown) {
      const result = errorMessageOf(error);
      setErrorMessage(result.message);
      setRequestID(result.requestID);
    } finally {
      if (abortControllerRef.current === abortController) {
        abortControllerRef.current = null;
      }

      setSubmissionPhase("idle");
      setIsCancelling(false);
    }
  }

  // 実行中のStorage Uploadを中断
  function handleCancel(): void {
    if (!isUploading || abortControllerRef.current === null) {
      return;
    }

    setIsCancelling(true);
    abortControllerRef.current.abort();
  }

  const titleLength = countCharacters(title);
  const descriptionLength = countCharacters(description);

  return (
    <main className="relative min-h-dvh overflow-hidden bg-[#100b08] px-4 py-6 text-stone-100 sm:px-6 lg:px-8">
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[radial-gradient(circle_at_16%_18%,rgba(217,119,6,0.2),transparent_30%),radial-gradient(circle_at_84%_78%,rgba(120,53,15,0.25),transparent_35%)]"
      />

      <div className="relative mx-auto w-full max-w-6xl">
        <header className="flex flex-col gap-5 border-b border-white/10 pb-6 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
              Coffee Reel Upload
            </p>
            <h1 className="mt-3 text-3xl font-black tracking-[-0.04em] text-white sm:text-5xl">
              動画を投稿
            </h1>
          </div>

          <Link
            to="/me/videos"
            className="inline-flex min-h-11 items-center justify-center rounded-full border border-white/15 px-5 py-2 text-sm font-black text-stone-200 transition hover:border-white/30 hover:bg-white/[0.06]"
          >
            自分の投稿へ
          </Link>
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

        <form
          className="mt-7 grid gap-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(420px,1.1fr)]"
          onSubmit={handleSubmit}
        >
          <section className="rounded-[2rem] border border-white/10 bg-white/[0.05] p-5 sm:p-7">
            <p className="text-xs font-black tracking-[0.18em] text-stone-500 uppercase">
              Video preview
            </p>

            <div className="mt-4 aspect-[9/16] max-h-[68vh] overflow-hidden rounded-[1.75rem] border border-white/10 bg-black/40">
              {previewURL !== "" && selectedVideo !== null ? (
                <video
                  src={previewURL}
                  controls
                  muted
                  playsInline
                  preload="metadata"
                  className="h-full w-full object-cover"
                />
              ) : (
                <div className="grid h-full place-items-center px-6 text-center">
                  <div>
                    <p className="text-4xl" aria-hidden="true">
                      🎬
                    </p>
                    <p className="mt-4 text-sm font-black text-stone-300">
                      動画を選択するとPreviewを表示
                    </p>
                    <p className="mt-2 text-xs leading-6 text-stone-500">
                      MP4 / MOV・最大10秒・最大50MB・1080×1920以下・9:16
                    </p>
                  </div>
                </div>
              )}
            </div>

            {selectedVideo !== null && (
              <dl className="mt-4 grid grid-cols-2 gap-3 text-xs">
                <div className="rounded-2xl border border-white/10 bg-black/20 p-3">
                  <dt className="font-bold text-stone-500">再生時間</dt>
                  <dd className="mt-1 font-black text-white">
                    {selectedVideo.metadata.durationSeconds.toFixed(1)}秒
                  </dd>
                </div>
                <div className="rounded-2xl border border-white/10 bg-black/20 p-3">
                  <dt className="font-bold text-stone-500">表示サイズ</dt>
                  <dd className="mt-1 font-black text-white">
                    {selectedVideo.metadata.width} ×{" "}
                    {selectedVideo.metadata.height}
                  </dd>
                </div>
              </dl>
            )}
          </section>

          <section className="rounded-[2rem] border border-white/10 bg-white/[0.05] p-5 sm:p-7">
            <div>
              <label
                htmlFor="video-file"
                className="text-sm font-black text-stone-200"
              >
                動画ファイル
              </label>
              <input
                id="video-file"
                type="file"
                accept=".mp4,.mov,video/mp4,video/quicktime"
                onChange={handleFileChange}
                disabled={isFormLocked}
                className="mt-2 block w-full rounded-2xl border border-dashed border-amber-300/30 bg-amber-300/[0.06] px-4 py-4 text-sm font-bold text-stone-300 file:mr-4 file:rounded-full file:border-0 file:bg-amber-300 file:px-4 file:py-2 file:text-xs file:font-black file:text-stone-950 hover:border-amber-300/60 disabled:cursor-not-allowed disabled:opacity-60"
              />
            </div>

            <div className="mt-5">
              <label
                htmlFor="video-title"
                className="text-sm font-black text-stone-200"
              >
                タイトル
              </label>
              <input
                id="video-title"
                type="text"
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                maxLength={maxTitleLength}
                disabled={isFormLocked}
                className={inputClass}
                placeholder="ハンドドリップの蒸らし方"
              />
              <p className="mt-2 text-right text-xs text-stone-500">
                {titleLength} / {maxTitleLength}
              </p>
            </div>

            <div className="mt-5">
              <label
                htmlFor="video-description"
                className="text-sm font-black text-stone-200"
              >
                説明
              </label>
              <textarea
                id="video-description"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                maxLength={maxDescriptionLength}
                disabled={isFormLocked}
                rows={5}
                className={`${inputClass} resize-y`}
                placeholder="動画のポイントや使用した器具を入力"
              />
              <p className="mt-2 text-right text-xs text-stone-500">
                {descriptionLength} / {maxDescriptionLength}
              </p>
            </div>

            <div className="mt-5">
              <label
                htmlFor="video-category"
                className="text-sm font-black text-stone-200"
              >
                カテゴリー
              </label>
              <select
                id="video-category"
                value={category}
                onChange={(event) =>
                  setCategory(event.target.value as CategoryCode)
                }
                disabled={isFormLocked}
                className={inputClass}
              >
                {categoryOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            {isUploading && selectedVideo !== null && (
              <div className="mt-6">
                <UploadProgress
                  fileName={selectedVideo.file.name}
                  progress={progress}
                  onCancel={handleCancel}
                  isCancelling={isCancelling}
                />
              </div>
            )}

            <div className="mt-6 rounded-2xl border border-sky-300/20 bg-sky-400/[0.07] p-4 text-sm leading-6 text-sky-100">
              {pendingCompletionVideoID === null
                ? "Upload完了後、自分の投稿画面へ移動します。"
                : "動画本体のUploadは完了しています。完了通知だけを再送してください。"}
            </div>

            <button
              type="submit"
              disabled={isSubmitting}
              className="mt-6 inline-flex min-h-12 w-full items-center justify-center rounded-full bg-amber-300 px-6 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submissionPhase === "uploading"
                ? "アップロード中"
                : submissionPhase === "completing"
                  ? "完了通知中"
                  : pendingCompletionVideoID !== null
                    ? "完了通知を再送"
                    : "動画を投稿"}
            </button>
          </section>
        </form>
      </div>
    </main>
  );
}
