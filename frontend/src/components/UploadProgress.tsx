import type { VideoUploadProgress } from "../api/upload";

type UploadProgressProps = {
  fileName: string;
  progress: VideoUploadProgress;
  onCancel?: () => void;
  isCancelling?: boolean;
};

// Bytes数を画面表示用の読みやすい単位へ変換
function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB"];
  const unitIndex = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  const size = value / 1024 ** unitIndex;

  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

// Progress値を表示可能な0〜100の範囲へ補正
function normalizePercentage(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }

  return Math.min(Math.max(Math.round(value), 0), 100);
}

// 動画Uploadの進捗、転送量、中断操作を表示
export default function UploadProgress({
  fileName,
  progress,
  onCancel,
  isCancelling = false,
}: UploadProgressProps) {
  const percentage = normalizePercentage(progress.percentage);
  const isComplete = percentage === 100;

  return (
    <section
      className="rounded-[1.75rem] border border-amber-300/20 bg-white/[0.055] p-5 shadow-xl shadow-black/10 sm:p-6"
      aria-label="動画アップロード進捗"
      aria-live="polite"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-[11px] font-black tracking-[0.2em] text-amber-300 uppercase">
            Upload progress
          </p>

          <h2 className="mt-2 truncate text-lg font-black text-white sm:text-xl">
            {fileName}
          </h2>
        </div>

        <span className="shrink-0 text-2xl font-black tabular-nums text-amber-300 sm:text-3xl">
          {percentage}%
        </span>
      </div>

      <div
        className="mt-5 h-3 overflow-hidden rounded-full bg-black/35"
        role="progressbar"
        aria-label={`${fileName}のアップロード進捗`}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percentage}
        aria-valuetext={`${percentage}%`}
      >
        <div
          className="h-full rounded-full bg-amber-300 transition-[width] duration-200 ease-out"
          style={{ width: `${percentage}%` }}
        />
      </div>

      <div className="mt-3 flex flex-col gap-3 text-sm sm:flex-row sm:items-center sm:justify-between">
        <p
          className="font-bold text-stone-300"
          aria-label="アップロード済みの容量"
        >
          {formatBytes(progress.loadedBytes)} /{" "}
          {formatBytes(progress.totalBytes)}
        </p>

        {isComplete ? (
          <p className="font-black text-emerald-300">アップロード完了</p>
        ) : onCancel !== undefined ? (
          <button
            type="button"
            onClick={onCancel}
            disabled={isCancelling}
            className="inline-flex min-h-10 items-center justify-center rounded-full border border-white/15 px-4 py-2 text-xs font-black text-stone-200 transition hover:border-red-300/50 hover:bg-red-400/10 hover:text-red-100 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isCancelling ? "中断中" : "アップロードを中断"}
          </button>
        ) : null}
      </div>
    </section>
  );
}
