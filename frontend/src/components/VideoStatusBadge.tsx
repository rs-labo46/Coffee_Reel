import type { VideoProcessingStatus, VideoPublishStatus } from "../types/video";

type VideoStatusBadgeProps = {
  processingStatus: VideoProcessingStatus;
  publishStatus: VideoPublishStatus;
};

type StatusView = {
  label: string;
  className: string;
};

// 処理状態と公開状態から利用者向けの表示内容を決定
function statusViewOf(
  processingStatus: VideoProcessingStatus,
  publishStatus: VideoPublishStatus,
): StatusView {
  switch (processingStatus) {
    case "uploading":
      return {
        label: "アップロード中",
        className: "border-amber-300/30 bg-amber-300/10 text-amber-200",
      };

    case "uploaded":
      return {
        label: "処理待ち",
        className: "border-sky-300/30 bg-sky-300/10 text-sky-200",
      };

    case "processing":
      return {
        label: "動画を処理中",
        className: "border-blue-300/30 bg-blue-300/10 text-blue-200",
      };

    case "expired":
      return {
        label: "アップロード期限切れ",
        className: "border-red-300/30 bg-red-300/10 text-red-200",
      };

    case "failed":
      return {
        label: "動画処理失敗",
        className: "border-red-300/30 bg-red-300/10 text-red-200",
      };

    case "ready":
      if (publishStatus === "published") {
        return {
          label: "公開中",
          className: "border-emerald-300/30 bg-emerald-300/10 text-emerald-200",
        };
      }

      if (publishStatus === "hidden") {
        return {
          label: "管理者により非公開",
          className: "border-red-300/30 bg-red-300/10 text-red-200",
        };
      }

      return {
        label: "非公開",
        className: "border-stone-300/20 bg-white/[0.06] text-stone-300",
      };
  }
}

// 動画の現在状態を1つのBadgeへまとめて表示
export default function VideoStatusBadge({
  processingStatus,
  publishStatus,
}: VideoStatusBadgeProps) {
  const view = statusViewOf(processingStatus, publishStatus);

  return (
    <span
      className={`inline-flex min-h-7 items-center rounded-full border px-3 py-1 text-[11px] font-black ${view.className}`}
      aria-label={`動画状態: ${view.label}`}
    >
      {view.label}
    </span>
  );
}
