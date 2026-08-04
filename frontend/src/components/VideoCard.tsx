import type { ReactNode } from "react";
import { Link } from "react-router";

import type { CategoryCode, OwnedVideo, PublicVideo } from "../types/video";
import VideoStatusBadge from "./VideoStatusBadge";

type VideoCardProps = {
  video: PublicVideo | OwnedVideo;
  to?: string;
  action?: ReactNode;
};

// Videoが投稿者向け一覧Responseか判定
function isOwnedVideo(video: PublicVideo | OwnedVideo): video is OwnedVideo {
  return "processing_status" in video;
}

// Category Codeを日本語表示へ変換
function categoryLabelOf(category: CategoryCode): string {
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

// ISO日時を日本語の画面表示へ変換
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

// ThumbnailまたはPlaceholderを表示
function VideoThumbnail({
  thumbnailURL,
  title,
}: {
  thumbnailURL: string | null;
  title: string;
}) {
  if (thumbnailURL === null || thumbnailURL === "") {
    return (
      <div className="grid aspect-[9/16] h-full w-full place-items-center bg-[radial-gradient(circle_at_35%_30%,rgba(251,191,36,0.16),transparent_30%),#19110d] px-5 text-center">
        <div>
          <p className="text-3xl" aria-hidden="true">
            ☕
          </p>
          <p className="mt-3 text-xs font-black text-stone-400">
            Thumbnail準備中
          </p>
        </div>
      </div>
    );
  }

  return (
    <img
      src={thumbnailURL}
      alt={`${title}のサムネイル`}
      loading="lazy"
      className="aspect-[9/16] h-full w-full object-cover transition duration-300 group-hover:scale-[1.02]"
    />
  );
}

// 公開動画または自分の投稿を一覧用Cardとして表示
export default function VideoCard({ video, to, action }: VideoCardProps) {
  const ownedVideo = isOwnedVideo(video);
  const thumbnailURL = video.thumbnail_url;

  const thumbnail = (
    <div className="group relative aspect-[9/16] overflow-hidden rounded-[1.5rem] border border-white/10 bg-black/30">
      <VideoThumbnail thumbnailURL={thumbnailURL} title={video.title} />
      <span className="absolute left-3 top-3 rounded-full bg-black/65 px-3 py-1 text-[10px] font-black text-white backdrop-blur-sm">
        {categoryLabelOf(video.category)}
      </span>
    </div>
  );

  return (
    <article className="grid gap-4 rounded-[1.75rem] border border-white/10 bg-white/[0.05] p-4 shadow-xl shadow-black/10 sm:grid-cols-[132px_minmax(0,1fr)]">
      {to === undefined ? thumbnail : <Link to={to}>{thumbnail}</Link>}

      <div className="flex min-w-0 flex-col">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            {to === undefined ? (
              <h2 className="line-clamp-2 text-lg font-black text-white">
                {video.title}
              </h2>
            ) : (
              <Link
                to={to}
                className="line-clamp-2 text-lg font-black text-white transition hover:text-amber-200"
              >
                {video.title}
              </Link>
            )}

            {!ownedVideo && (
              <p className="mt-2 text-sm font-bold text-stone-400">
                {video.author.name}
              </p>
            )}
          </div>

          {ownedVideo && (
            <VideoStatusBadge
              processingStatus={video.processing_status}
              publishStatus={video.publish_status}
            />
          )}
        </div>

        <p className="mt-3 text-xs font-bold text-stone-500">
          {formatDate(video.created_at)}
        </p>

        {!ownedVideo && video.description !== "" && (
          <p className="mt-3 line-clamp-3 text-sm leading-6 text-stone-300">
            {video.description}
          </p>
        )}

        {!ownedVideo && video.is_saved && (
          <p className="mt-3 text-xs font-black text-amber-300">保存済み</p>
        )}

        {action !== undefined && <div className="mt-auto pt-5">{action}</div>}
      </div>
    </article>
  );
}
