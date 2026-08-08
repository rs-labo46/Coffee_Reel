import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listReels } from "../api/video";
import { useAuth } from "../auth/useAuth";
import { authenticatedUser, guestUser, publicVideo } from "../tests/videoFixtures";
import type { PublicVideoListResponse } from "../types/video";
import SearchPage from "./SearchPage";

vi.mock("../api/video", () => ({
  listReels: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

vi.mock("../components/LikeButton", () => ({
  default: ({
    likeCount,
    isLiked,
  }: {
    likeCount: number;
    isLiked: boolean;
  }) => <p>{`Like ${likeCount} ${isLiked ? "liked" : "not-liked"}`}</p>,
}));

const listReelsMock = vi.mocked(listReels);
const useAuthMock = vi.mocked(useAuth);

function response(
  overrides: Partial<PublicVideoListResponse> = {},
): PublicVideoListResponse {
  return {
    items: [publicVideo()],
    next_cursor: null,
    has_more: false,
    result_type: "all",
    ...overrides,
  };
}

function renderPage(initialEntry = "/search") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SearchPage />
    </MemoryRouter>,
  );
}

describe("SearchPage", () => {
  beforeEach(() => {
    listReelsMock.mockReset();
    useAuthMock.mockReturnValue(guestUser());
  });

  it("初期表示で条件なし公開動画一覧を取得する", async () => {
    listReelsMock.mockResolvedValue(response());

    renderPage();

    expect(
      await screen.findByText("ハンドドリップの蒸らし方"),
    ).toBeInTheDocument();
    expect(listReelsMock).toHaveBeenCalledWith(
      {
        title: undefined,
        category: undefined,
        limit: 20,
      },
      expect.any(AbortSignal),
    );
  });

  it("URL QueryからTitleとCategoryをフォームへ復元する", async () => {
    listReelsMock.mockResolvedValue(
      response({ result_type: "matched" }),
    );

    renderPage("/search?title=%E7%84%99%E7%85%8E&category=roasting");

    expect(await screen.findByLabelText("タイトル")).toHaveValue("焙煎");
    expect(screen.getByLabelText("カテゴリー")).toHaveValue("roasting");
    expect(listReelsMock).toHaveBeenCalledWith(
      {
        title: "焙煎",
        category: "roasting",
        limit: 20,
      },
      expect.any(AbortSignal),
    );
  });

  it("Title検索をSubmitすると前後空白を除去してAPIへ送る", async () => {
    const user = userEvent.setup();
    listReelsMock
      .mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response({ result_type: "matched" }));

    renderPage();
    await screen.findByText("ハンドドリップの蒸らし方");

    await user.type(screen.getByLabelText("タイトル"), "  ドリップ  ");
    await user.click(screen.getByRole("button", { name: "検索" }));

    expect(listReelsMock).toHaveBeenLastCalledWith(
      {
        title: "ドリップ",
        category: undefined,
        limit: 20,
      },
      expect.any(AbortSignal),
    );
  });

  it("Category検索と複合検索で指定条件だけを送る", async () => {
    const user = userEvent.setup();
    listReelsMock.mockResolvedValue(response({ result_type: "matched" }));

    renderPage();
    await screen.findByText("ハンドドリップの蒸らし方");

    await user.selectOptions(screen.getByLabelText("カテゴリー"), "brewing");
    await user.click(screen.getByRole("button", { name: "検索" }));

    expect(listReelsMock).toHaveBeenLastCalledWith(
      {
        title: undefined,
        category: "brewing",
        limit: 20,
      },
      expect.any(AbortSignal),
    );

    await user.type(screen.getByLabelText("タイトル"), "蒸らし");
    await user.click(screen.getByRole("button", { name: "検索" }));

    expect(listReelsMock).toHaveBeenLastCalledWith(
      {
        title: "蒸らし",
        category: "brewing",
        limit: 20,
      },
      expect.any(AbortSignal),
    );
  });

  it("result_typeがsimilarなら案内文と類似結果を表示する", async () => {
    listReelsMock.mockResolvedValue(
      response({
        result_type: "similar",
        items: [publicVideo({ title: "ハンドドリップ入門" })],
      }),
    );

    renderPage("/search?title=%E3%83%8F%E3%83%B3%E3%83%89%E3%83%89%E3%83%AA%E3%83%83%E3%83%96%E3%83%97");

    expect(
      await screen.findByText(
        "一致する動画が見つからなかったため、近い動画を表示しています",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("ハンドドリップ入門")).toBeInTheDocument();
  });

  it("類似候補も0件なら無条件一覧へ差し替えず空状態を表示する", async () => {
    listReelsMock.mockResolvedValue(
      response({
        result_type: "similar",
        items: [],
      }),
    );

    renderPage("/search?title=notfound");

    expect(
      await screen.findByText("該当する動画はありません"),
    ).toBeInTheDocument();
    expect(listReelsMock).toHaveBeenCalledTimes(1);
  });

  it("次Pageは同じ検索条件とBackendのnext_cursorを変更せず送る", async () => {
    const user = userEvent.setup();
    listReelsMock
      .mockResolvedValueOnce(
        response({
          result_type: "matched",
          next_cursor: "opaque+/=cursor",
          has_more: true,
        }),
      )
      .mockResolvedValueOnce(
        response({
          items: [publicVideo({ id: 11, title: "次の動画" })],
          result_type: "matched",
        }),
      );

    renderPage("/search?title=drip&category=brewing");
    await screen.findByText("ハンドドリップの蒸らし方");

    await user.click(screen.getByRole("button", { name: "さらに読み込む" }));

    expect(await screen.findByText("次の動画")).toBeInTheDocument();
    expect(listReelsMock).toHaveBeenNthCalledWith(
      2,
      {
        title: "drip",
        category: "brewing",
        limit: 20,
        cursor: "opaque+/=cursor",
      },
      expect.any(AbortSignal),
    );
  });

  it("検索条件を変更したら古いCursorを破棄して先頭Pageから取得する", async () => {
    const user = userEvent.setup();
    listReelsMock
      .mockResolvedValueOnce(
        response({
          result_type: "matched",
          next_cursor: "old-cursor",
          has_more: true,
        }),
      )
      .mockResolvedValueOnce(response({ result_type: "matched" }));

    renderPage("/search?title=old");
    await screen.findByText("ハンドドリップの蒸らし方");

    const input = screen.getByLabelText("タイトル");
    await user.clear(input);
    await user.type(input, "new");
    await user.click(screen.getByRole("button", { name: "検索" }));

    expect(listReelsMock).toHaveBeenLastCalledWith(
      {
        title: "new",
        category: undefined,
        limit: 20,
      },
      expect.any(AbortSignal),
    );
  });

  it("古い検索Responseが後から完了しても最新結果を上書きしない", async () => {
    const user = userEvent.setup();
    let resolveOld: ((value: PublicVideoListResponse) => void) | undefined;
    let resolveNew: ((value: PublicVideoListResponse) => void) | undefined;

    listReelsMock
      .mockReturnValueOnce(
        new Promise<PublicVideoListResponse>((resolve) => {
          resolveOld = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise<PublicVideoListResponse>((resolve) => {
          resolveNew = resolve;
        }),
      );

    renderPage("/search?title=old");

    const input = await screen.findByLabelText("タイトル");
    await user.clear(input);
    await user.type(input, "new");
    await user.click(screen.getByRole("button", { name: "検索" }));

    await act(async () => {
      resolveNew?.(
        response({
          items: [publicVideo({ id: 20, title: "最新結果" })],
          result_type: "matched",
        }),
      );
    });
    expect(await screen.findByText("最新結果")).toBeInTheDocument();

    await act(async () => {
      resolveOld?.(
        response({
          items: [publicVideo({ id: 19, title: "古い結果" })],
          result_type: "matched",
        }),
      );
    });

    expect(screen.getByText("最新結果")).toBeInTheDocument();
    expect(screen.queryByText("古い結果")).not.toBeInTheDocument();
  });

  it("EnterでSubmitでき、101文字はClient側で拒否する", async () => {
    const user = userEvent.setup();
    listReelsMock.mockResolvedValue(response());

    renderPage();
    await screen.findByText("ハンドドリップの蒸らし方");

    const input = screen.getByLabelText("タイトル");
    await user.type(input, "drip{enter}");

    expect(listReelsMock).toHaveBeenCalledTimes(2);

    const updatedInput = await screen.findByDisplayValue("drip");
    await user.clear(updatedInput);
    await user.type(updatedInput, "あ".repeat(101));
    await user.click(screen.getByRole("button", { name: "検索" }));

    expect(
      screen.getByText("タイトルは100文字以内で入力してください"),
    ).toBeInTheDocument();
    expect(listReelsMock).toHaveBeenCalledTimes(2);
  });

  it("XSS文字列をHTMLとして実行せずTextとして表示する", async () => {
    const title = '<img src="x" onerror="alert(1)">';
    listReelsMock.mockResolvedValue(
      response({
        items: [publicVideo({ title })],
        result_type: "matched",
      }),
    );

    const { container } = renderPage("/search?title=xss");

    expect(await screen.findByText(title)).toBeInTheDocument();
    expect(container.querySelector('img[src="x"]')).toBeNull();
  });

  it("認証済みUserの検索結果ではBackendのLike状態を表示へ渡す", async () => {
    useAuthMock.mockReturnValue(authenticatedUser());
    listReelsMock.mockResolvedValue(
      response({
        items: [publicVideo({ like_count: 7, is_liked: true })],
        result_type: "matched",
      }),
    );

    renderPage("/search?title=drip");

    expect(await screen.findByText("Like 7 liked")).toBeInTheDocument();
  });
});
