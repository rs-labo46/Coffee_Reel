import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { signUp } from "../api/user";
import { useAuth } from "../auth/useAuth";
import SignupPage from "./SignupPage";

vi.mock("../api/user", () => ({
  signUp: vi.fn(),
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const signUpMock = vi.mocked(signUp);
const useAuthMock = vi.mocked(useAuth);
const loginMock = vi.fn();
const logoutMock = vi.fn();

function renderSignupPage() {
  render(
    <MemoryRouter initialEntries={["/signup"]}>
      <Routes>
        <Route path="/signup" element={<SignupPage />} />
        <Route path="/" element={<p>トップ遷移先</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

async function enterValidSignupInput() {
  const user = userEvent.setup();

  await user.type(screen.getByLabelText("名前"), "コーヒー太郎");
  await user.type(
    screen.getByLabelText("メールアドレス"),
    "coffee@example.com",
  );
  await user.type(screen.getByLabelText("パスワード"), "password123");

  return user;
}

describe("SignupPage", () => {
  beforeEach(() => {
    signUpMock.mockReset();
    loginMock.mockReset();
    logoutMock.mockReset();
    loginMock.mockResolvedValue(undefined);
    useAuthMock.mockReturnValue({
      user: null,
      accessToken: null,
      isAuthenticated: false,
      isLoading: false,
      login: loginMock,
      logout: logoutMock,
    });
    renderSignupPage();
  });

  it("入力内容を送信し、自動ログイン後にトップ画面へ遷移する", async () => {
    signUpMock.mockResolvedValue({
      data: {
        id: 1,
        name: "コーヒー太郎",
        email: "coffee@example.com",
        role: "user",
        status: "active",
        created_at: "2026-07-21T00:00:00Z",
      },
    });

    const user = await enterValidSignupInput();
    await user.click(screen.getByRole("button", { name: "登録する" }));

    expect(signUpMock).toHaveBeenCalledWith({
      name: "コーヒー太郎",
      email: "coffee@example.com",
      password: "password123",
    });
    expect(loginMock).toHaveBeenCalledWith({
      email: "coffee@example.com",
      password: "password123",
    });
    expect(await screen.findByText("トップ遷移先")).toBeInTheDocument();
  });

  it("送信中は登録ボタンを無効化し、二重送信を防ぐ", async () => {
    let resolveRequest: (() => void) | undefined;

    signUpMock.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveRequest = () =>
            resolve({
              data: {
                id: 1,
                name: "コーヒー太郎",
                email: "coffee@example.com",
                role: "user",
                status: "active",
                created_at: "2026-07-21T00:00:00Z",
              },
            });
        }),
    );

    const user = await enterValidSignupInput();
    const submitButton = screen.getByRole("button", { name: "登録する" });

    await user.click(submitButton);

    expect(screen.getByRole("button", { name: "登録中..." })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "登録中..." }));
    expect(signUpMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveRequest?.();
    });
    expect(await screen.findByText("トップ遷移先")).toBeInTheDocument();
  });

  it("APIエラーを画面へ表示する", async () => {
    signUpMock.mockRejectedValue(
      new ApiClientError(
        409,
        "email_already_exists",
        "このメールアドレスは既に登録されています",
        "req-signup",
      ),
    );

    const user = await enterValidSignupInput();
    await user.click(screen.getByRole("button", { name: "登録する" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "このメールアドレスは既に登録されています",
    );
    expect(loginMock).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "登録する" })).toBeEnabled();
  });

  it("UTF-8で72バイトを超えるPasswordはAPI送信前に拒否する", async () => {
    const user = userEvent.setup();

    await user.type(screen.getByLabelText("名前"), "コーヒー太郎");
    await user.type(
      screen.getByLabelText("メールアドレス"),
      "coffee@example.com",
    );
    await user.type(screen.getByLabelText("パスワード"), "あ".repeat(25));
    await user.click(screen.getByRole("button", { name: "登録する" }));

    expect(screen.getByRole("alert")).toHaveTextContent(
      "パスワードはUTF-8で72バイト以内にしてください",
    );
    expect(signUpMock).not.toHaveBeenCalled();
  });
});
