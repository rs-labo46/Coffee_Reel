import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { useAuth } from "../auth/useAuth";
import LoginPage from "./LoginPage";

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);
const loginMock =
  vi.fn<(input: { email: string; password: string }) => Promise<void>>();

function renderLoginPage(initialEntry = "/login") {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/" element={<p>ホーム遷移先</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

async function enterLoginInput() {
  const user = userEvent.setup();

  await user.type(
    screen.getByLabelText("メールアドレス"),
    "coffee@example.com",
  );
  await user.type(screen.getByLabelText("パスワード"), "password123");

  return user;
}

describe("LoginPage", () => {
  beforeEach(() => {
    loginMock.mockReset();
    useAuthMock.mockReturnValue({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isLoading: false,
      login: loginMock,
      logout: vi.fn(),
    });
  });

  it("正常Login後にHome画面へ遷移する", async () => {
    loginMock.mockResolvedValue(undefined);

    renderLoginPage();

    const user = await enterLoginInput();
    await user.click(screen.getByRole("button", { name: "ログイン" }));

    expect(loginMock).toHaveBeenCalledWith({
      email: "coffee@example.com",
      password: "password123",
    });
    expect(await screen.findByText("ホーム遷移先")).toBeInTheDocument();
  });

  it("認証失敗を画面へ表示する", async () => {
    loginMock.mockRejectedValue(
      new ApiClientError(
        401,
        "invalid_credentials",
        "メールアドレスまたはパスワードが正しくありません",
        "req-login",
      ),
    );

    renderLoginPage();

    const user = await enterLoginInput();
    await user.click(screen.getByRole("button", { name: "ログイン" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "メールアドレスまたはパスワードが正しくありません",
    );
    expect(screen.getByRole("button", { name: "ログイン" })).toBeEnabled();
  });

  it("Rate Limitエラーを画面へ表示する", async () => {
    loginMock.mockRejectedValue(
      new ApiClientError(
        429,
        "rate_limit_exceeded",
        "リクエスト回数が上限を超えました",
        "req-rate-limit",
      ),
    );

    renderLoginPage();

    const user = await enterLoginInput();
    await user.click(screen.getByRole("button", { name: "ログイン" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "リクエスト回数が上限を超えました",
    );
  });

  it("会員登録完了後は完了メッセージを表示する", () => {
    render(
      <MemoryRouter
        initialEntries={[
          {
            pathname: "/login",
            state: {
              registrationCompleted: true,
            },
          },
        ]}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByRole("status")).toHaveTextContent(
      "会員登録が完了しました",
    );
  });
  it("LikeからLoginした場合は認証成功後に元の相対URLへ戻る", async () => {
    loginMock.mockResolvedValue(undefined);

    render(
      <MemoryRouter
        initialEntries={[
          "/login?redirect=%2Fsearch%3Ftitle%3Ddrip%26category%3Dbrewing",
        ]}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/search" element={<p>検索遷移先</p>} />
        </Routes>
      </MemoryRouter>,
    );

    const user = await enterLoginInput();
    await user.click(screen.getByRole("button", { name: "ログイン" }));

    expect(await screen.findByText("検索遷移先")).toBeInTheDocument();
  });

  it("外部URLやProtocol-relative URLはLogin後の遷移先に使用しない", async () => {
    loginMock.mockResolvedValue(undefined);

    render(
      <MemoryRouter
        initialEntries={["/login?redirect=%2F%2Fevil.example%2Fpath"]}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<p>ホーム遷移先</p>} />
        </Routes>
      </MemoryRouter>,
    );

    const user = await enterLoginInput();
    await user.click(screen.getByRole("button", { name: "ログイン" }));

    expect(await screen.findByText("ホーム遷移先")).toBeInTheDocument();
  });
});
