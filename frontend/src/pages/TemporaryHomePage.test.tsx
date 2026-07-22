import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { useAuth } from "../auth/useAuth";
import TemporaryHomePage from "./TemporaryHomePage";

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);
const logoutMock = vi.fn<() => Promise<void>>();

describe("TemporaryHomePage", () => {
  beforeEach(() => {
    logoutMock.mockReset();

    useAuthMock.mockReturnValue({
      user: {
        id: 1,
        name: "コーヒー太郎",
        email: "coffee@example.com",
        role: "user",
        status: "active",
      },
      accessToken: "access-token",
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: logoutMock,
    });
  });

  it("Login Userの名前、Email、Roleを表示する", () => {
    render(<TemporaryHomePage />);

    expect(screen.getByText("コーヒー太郎さん、ようこそ。")).toBeInTheDocument();
    expect(screen.getByText("coffee@example.com")).toBeInTheDocument();
    expect(screen.getByText("user")).toBeInTheDocument();
  });

  it("LogoutボタンからAuthContextのLogoutを実行する", async () => {
    logoutMock.mockResolvedValue(undefined);

    render(<TemporaryHomePage />);

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "ログアウト" }));

    expect(logoutMock).toHaveBeenCalledTimes(1);
  });

  it("Logout失敗時はエラーを表示する", async () => {
    logoutMock.mockRejectedValue(
      new ApiClientError(
        403,
        "csrf_invalid",
        "CSRFトークンが正しくありません",
        "req-logout",
      ),
    );

    render(<TemporaryHomePage />);

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "ログアウト" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "CSRFトークンが正しくありません",
    );
    expect(screen.getByRole("button", { name: "ログアウト" })).toBeEnabled();
  });
});
