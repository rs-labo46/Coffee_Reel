import { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
} from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { likeVideo, unlikeVideo } from "../api/video_like";
import type { VideoLikeState } from "../types/video";
import LikeButton from "./LikeButton";

vi.mock("../api/video_like", () => ({
  likeVideo: vi.fn(),
  unlikeVideo: vi.fn(),
}));

const likeVideoMock = vi.mocked(likeVideo);
const unlikeVideoMock = vi.mocked(unlikeVideo);

function LoginProbe() {
  const location = useLocation();
  const redirect = new URLSearchParams(location.search).get("redirect") ?? "";

  return <p>LoginPage redirect={redirect}</p>;
}

function LikeHarness({
  initialCount = 3,
  initialLiked = false,
  isAuthenticated = true,
  onNotFound,
  onError,
}: {
  initialCount?: number;
  initialLiked?: boolean;
  isAuthenticated?: boolean;
  onNotFound?: () => void;
  onError?: (error: unknown) => void;
}) {
  const [state, setState] = useState<VideoLikeState>({
    video_id: 10,
    like_count: initialCount,
    is_liked: initialLiked,
  });

  return (
    <LikeButton
      videoID={10}
      likeCount={state.like_count}
      isLiked={state.is_liked}
      isAuthenticated={isAuthenticated}
      onChange={setState}
      onNotFound={onNotFound}
      onError={onError}
    />
  );
}

function renderButton(
  element: React.ReactNode,
  initialEntry = "/search?title=drip",
) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/search" element={element} />
        <Route path="/videos/:video_id" element={element} />
        <Route path="/login" element={<LoginProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("LikeButton", () => {
  beforeEach(() => {
    likeVideoMock.mockReset();
    unlikeVideoMock.mockReset();
  });

  it("Guestは件数を表示し、APIを呼ばずLoginへ元の相対URLを渡す", async () => {
    const user = userEvent.setup();

    renderButton(<LikeHarness isAuthenticated={false} initialCount={12} />);

    const button = screen.getByRole("button", {
      name: "ログインしていいね 12件",
    });
    expect(button).toHaveAttribute("aria-pressed", "false");

    await user.click(button);

    expect(likeVideoMock).not.toHaveBeenCalled();
    expect(unlikeVideoMock).not.toHaveBeenCalled();
    expect(
      screen.getByText("LoginPage redirect=/search?title=drip"),
    ).toBeInTheDocument();
  });

  it("未LikeならPUTを呼びResponseの件数と状態をそのまま反映する", async () => {
    const user = userEvent.setup();
    likeVideoMock.mockResolvedValue({
      video_id: 10,
      like_count: 4,
      is_liked: true,
    });

    renderButton(<LikeHarness />);

    await user.click(screen.getByRole("button", { name: "いいね 3件" }));

    expect(likeVideoMock).toHaveBeenCalledWith(10);
    expect(unlikeVideoMock).not.toHaveBeenCalled();
    expect(
      await screen.findByRole("button", { name: "いいねを解除 4件" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("Like済みならDELETEを呼び解除後状態を反映する", async () => {
    const user = userEvent.setup();
    unlikeVideoMock.mockResolvedValue({
      video_id: 10,
      like_count: 2,
      is_liked: false,
    });

    renderButton(<LikeHarness initialLiked initialCount={3} />);

    await user.click(
      screen.getByRole("button", { name: "いいねを解除 3件" }),
    );

    expect(unlikeVideoMock).toHaveBeenCalledWith(10);
    expect(likeVideoMock).not.toHaveBeenCalled();
    expect(
      await screen.findByRole("button", { name: "いいね 2件" }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  it("処理中はButtonを無効化して二重送信を防ぐ", async () => {
    const user = userEvent.setup();
    let resolveLike: ((value: VideoLikeState) => void) | undefined;
    likeVideoMock.mockReturnValue(
      new Promise<VideoLikeState>((resolve) => {
        resolveLike = resolve;
      }),
    );

    renderButton(<LikeHarness />);

    const button = screen.getByRole("button", { name: "いいね 3件" });
    await user.click(button);

    expect(screen.getByRole("button", { name: "いいね 3件" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "いいね 3件" }));
    expect(likeVideoMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveLike?.({ video_id: 10, like_count: 4, is_liked: true });
    });
    expect(
      await screen.findByRole("button", { name: "いいねを解除 4件" }),
    ).toBeEnabled();
  });

  it("API失敗時は元の表示状態を維持してErrorを親へ渡す", async () => {
    const user = userEvent.setup();
    const onError = vi.fn();
    const error = new ApiClientError(
      500,
      "internal_error",
      "更新できません",
      "request-like-1",
    );
    likeVideoMock.mockRejectedValue(error);

    renderButton(<LikeHarness onError={onError} />);

    await user.click(screen.getByRole("button", { name: "いいね 3件" }));

    expect(onError).toHaveBeenCalledWith(error);
    expect(
      await screen.findByRole("button", { name: "いいね 3件" }),
    ).toHaveAttribute("aria-pressed", "false");
  });

  it("404は対象Videoが非公開化された扱いとして親へ通知する", async () => {
    const user = userEvent.setup();
    const onNotFound = vi.fn();
    const onError = vi.fn();
    likeVideoMock.mockRejectedValue(
      new ApiClientError(404, "video_not_found", "見つかりません", "request-404"),
    );

    renderButton(
      <LikeHarness onNotFound={onNotFound} onError={onError} />,
      "/videos/10",
    );

    await user.click(screen.getByRole("button", { name: "いいね 3件" }));

    expect(onNotFound).toHaveBeenCalledTimes(1);
    expect(onError).not.toHaveBeenCalled();
  });
});
