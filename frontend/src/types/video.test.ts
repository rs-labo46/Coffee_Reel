import { describe, expect, expectTypeOf, it } from "vitest";

import type {
  PublicSearchResultType,
  PublicVideo,
  VideoProcessingStatus,
} from "./video";

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
  it("公開動画へLike件数と本人Like状態を固定型で保持する", () => {
    expectTypeOf<PublicVideo["like_count"]>().toEqualTypeOf<number>();
    expectTypeOf<PublicVideo["is_liked"]>().toEqualTypeOf<boolean>();
  });

  it("公開検索Result Typeをall・matched・similarへ限定する", () => {
    expectTypeOf<PublicSearchResultType>().toEqualTypeOf<
      "all" | "matched" | "similar"
    >();
  });
});
