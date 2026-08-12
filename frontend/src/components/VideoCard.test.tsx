import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";

import { ownedVideo, publicVideo } from "../tests/videoFixtures";
import VideoCard from "./VideoCard";

describe("VideoCard", () => {
  it("公開動画の投稿者・説明・保存状態・詳細Linkを表示する", () => {
    render(
      <MemoryRouter>
        <VideoCard
          video={publicVideo({ is_saved: true })}
          to="/videos/10"
        />
      </MemoryRouter>,
    );

    expect(
      screen.getByRole("link", { name: "コーヒー太郎の公開動画を見る" }),
    ).toHaveAttribute("href", "/videos/author/1");
    expect(
      screen.getByText("30秒蒸らしてからゆっくり注ぎます"),
    ).toBeInTheDocument();
    const savedStatus = screen.getByLabelText("保存済み");
    expect(savedStatus).toHaveTextContent("");
    expect(savedStatus.querySelector("svg")).not.toBeNull();
    expect(
      screen.getByRole("link", { name: "ハンドドリップの蒸らし方" }),
    ).toHaveAttribute("href", "/videos/10");
  });

  it("自分の投稿へ状態Badgeと操作領域を表示する", () => {
    render(
      <MemoryRouter>
        <VideoCard
          video={ownedVideo({
            processing_status: "ready",
            publish_status: "hidden",
          })}
          action={<button type="button">削除</button>}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText("管理者により非公開")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "削除" })).toBeInTheDocument();
    expect(screen.getByText("Thumbnail準備中")).toBeInTheDocument();
  });

  it("TitleとDescription内のHTML文字列を実行せず文字として表示する", () => {
    const title = "<script>window.hacked=true</script>";
    const description = '<img src=x onerror="window.hacked=true">';
    const { container } = render(
      <MemoryRouter>
        <VideoCard
          video={publicVideo({
            title,
            description,
          })}
        />
      </MemoryRouter>,
    );

    expect(screen.getByText(title)).toBeInTheDocument();
    expect(screen.getByText(description)).toBeInTheDocument();
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector('img[src="x"]')).toBeNull();
  });
});
