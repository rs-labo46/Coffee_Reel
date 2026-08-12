import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { publicVideo } from "../tests/videoFixtures";
import ReelVideo from "./ReelVideo";

const baseProps = {
  video: publicVideo(),
  isActive: false,
  shouldPreload: false,
  isAuthenticated: true,
  isSaving: false,
  onVisibilityChange: vi.fn(),
  onToggleSaved: vi.fn(),
  onLikeChange: vi.fn(),
  onLikeNotFound: vi.fn(),
  onLikeError: vi.fn(),
};

// ReelVideoをRouter内で描画
function renderVideo(
  overrides: Partial<React.ComponentProps<typeof ReelVideo>> = {},
) {
  return render(
    <MemoryRouter>
      <ReelVideo {...baseProps} {...overrides} />
    </MemoryRouter>,
  );
}

describe("ReelVideo", () => {
  beforeEach(() => {
    vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(
      () => undefined,
    );
    vi.spyOn(HTMLMediaElement.prototype, "load").mockImplementation(
      () => undefined,
    );
  });

  it("Active動画だけを自動再生する", () => {
    renderVideo({ isActive: true });

    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1);
    expect(HTMLMediaElement.prototype.pause).not.toHaveBeenCalled();
  });

  it("画面外の動画を停止してMedia URLを解除する", () => {
    const { container } = renderVideo({
      isActive: false,
      shouldPreload: false,
    });
    const videoElement =
      screen.getByLabelText("ハンドドリップの蒸らし方を再生");

    expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled();
    expect(HTMLMediaElement.prototype.load).toHaveBeenCalled();
    expect(videoElement).not.toHaveAttribute("src");
    expect(container.querySelector("video")).toHaveAttribute("preload", "none");
  });

  it("次の1件だけをmetadata先読み対象として保持する", () => {
    renderVideo({
      isActive: false,
      shouldPreload: true,
    });

    const videoElement =
      screen.getByLabelText("ハンドドリップの蒸らし方を再生");

    expect(videoElement).toHaveAttribute(
      "src",
      "https://storage.example.com/video.mp4",
    );
    expect(videoElement).toHaveAttribute("preload", "metadata");
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it("投稿者名から投稿者の公開動画一覧へ移動できる", () => {
    renderVideo();

    expect(
      screen.getByRole("link", { name: "コーヒー太郎の公開動画を見る" }),
    ).toHaveAttribute("href", "/videos/author/1");
  });

  it("音声ON/OFFをSVGアイコンだけで切り替える", async () => {
    const user = userEvent.setup();

    renderVideo();

    const videoElement =
      screen.getByLabelText("ハンドドリップの蒸らし方を再生");
    const mutedButton = screen.getByRole("button", { name: "音声をオン" });

    expect(videoElement).toHaveProperty("muted", true);
    expect(mutedButton).toHaveTextContent("");
    expect(mutedButton.querySelector("svg")).not.toBeNull();

    await user.click(mutedButton);

    const unmutedButton = screen.getByRole("button", { name: "音声をオフ" });
    expect(videoElement).toHaveProperty("muted", false);
    expect(unmutedButton).toHaveTextContent("");
    expect(unmutedButton.querySelector("svg")).not.toBeNull();
  });

  it("保存Buttonから対象動画を親処理へ渡す", async () => {
    const user = userEvent.setup();
    const onToggleSaved = vi.fn();
    const video = publicVideo();

    renderVideo({ video, onToggleSaved });

    const saveButton = screen.getByRole("button", { name: "保存" });
    expect(saveButton).toHaveTextContent("");
    expect(saveButton.querySelector("svg")).not.toBeNull();

    await user.click(saveButton);

    expect(onToggleSaved).toHaveBeenCalledWith(video);
  });

  it("未認証時も保存操作をIconだけで表示する", () => {
    renderVideo({ isAuthenticated: false });

    const saveButton = screen.getByRole("button", { name: "ログインして保存" });
    expect(saveButton).toHaveTextContent("");
    expect(saveButton.querySelector("svg")).not.toBeNull();
  });

  it("再生失敗時に再試行Buttonを表示する", async () => {
    const user = userEvent.setup();
    vi.mocked(HTMLMediaElement.prototype.play).mockRejectedValue(
      new Error("autoplay blocked"),
    );

    renderVideo({ isActive: true });

    const retryButton = await screen.findByRole("button", { name: "再試行" });
    await user.click(retryButton);

    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(2);
  });
  it("リールへLike件数と本人状態を表示する", () => {
    renderVideo({
      video: publicVideo({ like_count: 8, is_liked: true }),
    });

    expect(
      screen.getByRole("button", { name: "いいねを解除 8件" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
});
