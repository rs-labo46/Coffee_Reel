import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type {
  VideoProcessingStatus,
  VideoPublishStatus,
} from "../types/video";
import VideoStatusBadge from "./VideoStatusBadge";

type StatusCase = {
  processingStatus: VideoProcessingStatus;
  publishStatus: VideoPublishStatus;
  label: string;
};

const statusCases: StatusCase[] = [
  {
    processingStatus: "uploading",
    publishStatus: "private",
    label: "アップロード中",
  },
  {
    processingStatus: "uploaded",
    publishStatus: "private",
    label: "処理待ち",
  },
  {
    processingStatus: "processing",
    publishStatus: "private",
    label: "動画を処理中",
  },
  {
    processingStatus: "ready",
    publishStatus: "published",
    label: "公開中",
  },
  {
    processingStatus: "ready",
    publishStatus: "private",
    label: "非公開",
  },
  {
    processingStatus: "ready",
    publishStatus: "hidden",
    label: "管理者により非公開",
  },
  {
    processingStatus: "expired",
    publishStatus: "private",
    label: "アップロード期限切れ",
  },
  {
    processingStatus: "failed",
    publishStatus: "private",
    label: "動画処理失敗",
  },
];

describe("VideoStatusBadge", () => {
  it.each(statusCases)(
    "$processingStatus/$publishStatusを$labelとして表示する",
    ({ processingStatus, publishStatus, label }) => {
      render(
        <VideoStatusBadge
          processingStatus={processingStatus}
          publishStatus={publishStatus}
        />,
      );

      expect(screen.getByText(label)).toHaveAccessibleName(
        `動画状態: ${label}`,
      );
    },
  );
});
