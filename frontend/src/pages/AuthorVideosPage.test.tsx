import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listReels } from "../api/video";
import { useAuth } from "../auth/useAuth";
import { authenticatedUser } from "../tests/videoFixtures";
import { publicVideo } from "../tests/videoFixtures";
import AuthorVideosPage from "./AuthorVideosPage";

vi.mock("../api/video", () => ({
  listReels: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("../components/LikeButton", () => ({
  default: ({ videoID }: { videoID: number }) => <button>Like {videoID}</button>,
}));

const listReelsMock = vi.mocked(listReels);
const useAuthMock = vi.mocked(useAuth);

async function flushAsync(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  });
}

function renderAuthorPage(path = "/videos/author/7") {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/videos/author/:author_id" element={<AuthorVideosPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AuthorVideosPage", () => {
  beforeEach(() => {
    useAuthMock.mockReturnValue(authenticatedUser());
    listReelsMock.mockReset();
  });

  it("投稿者IDで公開動画を取得して投稿者名と動画一覧を表示する", async () => {
    listReelsMock.mockResolvedValue({
      items: [
        publicVideo({
          id: 30,
          author: { id: 7, name: "別の投稿者" },
        }),
      ],
      next_cursor: null,
      has_more: false,
    });

    renderAuthorPage();
    await flushAsync();

    expect(listReelsMock).toHaveBeenCalledWith({ author_id: 7 });
    expect(
      screen.getByRole("heading", { name: "別の投稿者" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "ハンドドリップの蒸らし方" }),
    ).toHaveAttribute("href", "/videos/30");
  });


  it("画面幅に関係なく投稿者動画を横3カードで表示する", async () => {
    listReelsMock.mockResolvedValue({
      items: [
        publicVideo({
          id: 30,
          title: "1本目",
          author: { id: 7, name: "別の投稿者" },
        }),
        publicVideo({
          id: 29,
          title: "2本目",
          author: { id: 7, name: "別の投稿者" },
        }),
        publicVideo({
          id: 28,
          title: "3本目",
          author: { id: 7, name: "別の投稿者" },
        }),
      ],
      next_cursor: null,
      has_more: false,
    });

    renderAuthorPage();
    await flushAsync();

    const videoList = screen.getByRole("region", {
      name: "投稿者の公開動画一覧",
    });

    expect(videoList).toHaveClass("grid-cols-3");
    expect(videoList).not.toHaveClass("md:grid-cols-2");
    expect(videoList).not.toHaveClass("lg:grid-cols-3");
    expect(screen.getByRole("link", { name: "1本目" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "2本目" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "3本目" })).toBeInTheDocument();
  });

  it("次Cursorでも同じ投稿者IDを維持する", async () => {
    const user = userEvent.setup();
    listReelsMock
      .mockResolvedValueOnce({
        items: [
          publicVideo({
            id: 30,
            author: { id: 7, name: "別の投稿者" },
          }),
        ],
        next_cursor: "next-cursor",
        has_more: true,
      })
      .mockResolvedValueOnce({
        items: [
          publicVideo({
            id: 29,
            title: "2本目",
            author: { id: 7, name: "別の投稿者" },
          }),
        ],
        next_cursor: null,
        has_more: false,
      });

    renderAuthorPage();
    await flushAsync();

    await user.click(screen.getByRole("button", { name: "さらに読み込む" }));
    await flushAsync();

    expect(listReelsMock).toHaveBeenNthCalledWith(2, {
      author_id: 7,
      cursor: "next-cursor",
    });
    expect(screen.getByRole("link", { name: "2本目" })).toBeInTheDocument();
  });

  it("不正な投稿者IDではAPIを呼ばない", async () => {
    renderAuthorPage("/videos/author/0");
    await flushAsync();

    expect(listReelsMock).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "投稿者IDが正しくありません",
    );
  });
});
