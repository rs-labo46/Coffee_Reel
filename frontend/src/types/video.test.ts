import { describe, expect, expectTypeOf, it } from "vitest";

import type { VideoProcessingStatus } from "./video";

const processingStatuses = [
  "uploading",
  "expired",
  "uploaded",
  "processing",
  "ready",
  "failed",
] as const satisfies readonly VideoProcessingStatus[];

describe("動画Frontend型", () => {
  it("deletedをProcessingStatusとして扱わない", () => {
    expect(processingStatuses).not.toContain("deleted");
    expectTypeOf<"deleted">().not.toMatchTypeOf<VideoProcessingStatus>();
  });
});
