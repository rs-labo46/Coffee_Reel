type BookmarkIconProps = {
  filled?: boolean;
  className?: string;
};

// 保存導線と保存状態で共通利用するBookmark SVG
export default function BookmarkIcon({
  filled = false,
  className = "h-5 w-5",
}: BookmarkIconProps) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      className={className}
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M6 4.5A1.5 1.5 0 0 1 7.5 3h9A1.5 1.5 0 0 1 18 4.5V21l-6-4-6 4V4.5Z" />
    </svg>
  );
}
