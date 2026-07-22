import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { refreshAccessToken } from "../api/client";
import {
  getMe,
  login as requestLogin,
  logout as requestLogout,
} from "../api/user";
import { AuthProvider } from "./AuthProvider";
import { useAuth } from "./useAuth";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();

  return {
    ...actual,
    refreshAccessToken: vi.fn(),
  };
});

vi.mock("../api/user", () => ({
  getMe: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

const refreshAccessTokenMock = vi.mocked(refreshAccessToken);
const getMeMock = vi.mocked(getMe);
const requestLoginMock = vi.mocked(requestLogin);
const requestLogoutMock = vi.mocked(requestLogout);

function AuthProbe() {
  const auth = useAuth();

  return (
    <div>
      <p data-testid="loading">{String(auth.isLoading)}</p>
      <p data-testid="authenticated">{String(auth.isAuthenticated)}</p>
      <p data-testid="token">{auth.accessToken ?? "none"}</p>
      <p data-testid="user">{auth.user?.email ?? "none"}</p>

      <button
        type="button"
        onClick={() =>
          void auth.login({
            email: "coffee@example.com",
            password: "password123",
          })
        }
      >
        Login
      </button>

      <button type="button" onClick={() => void auth.logout()}>
        Logout
      </button>
    </div>
  );
}

function renderAuthProvider() {
  render(
    <AuthProvider>
      <AuthProbe />
    </AuthProvider>,
  );
}

describe("AuthProvider", () => {
  beforeEach(() => {
    refreshAccessTokenMock.mockReset();
    getMeMock.mockReset();
    requestLoginMock.mockReset();
    requestLogoutMock.mockReset();
  });

  it("ページ読込時にRefreshとMeを実行して認証状態を復元する", async () => {
    refreshAccessTokenMock.mockResolvedValue("restored-token");
    getMeMock.mockResolvedValue({
      data: {
        id: 1,
        name: "コーヒー太郎",
        email: "coffee@example.com",
        role: "user",
        status: "active",
        created_at: "2026-07-21T00:00:00Z",
      },
    });

    renderAuthProvider();

    expect(screen.getByTestId("loading")).toHaveTextContent("true");

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    expect(refreshAccessTokenMock).toHaveBeenCalledTimes(1);
    expect(getMeMock).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("token")).toHaveTextContent("restored-token");
    expect(screen.getByTestId("user")).toHaveTextContent(
      "coffee@example.com",
    );
  });

  it("認証復元に失敗した場合は未認証状態へ初期化する", async () => {
    refreshAccessTokenMock.mockRejectedValue(new Error("unauthorized"));

    renderAuthProvider();

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });

    expect(screen.getByTestId("authenticated")).toHaveTextContent("false");
    expect(screen.getByTestId("token")).toHaveTextContent("none");
    expect(screen.getByTestId("user")).toHaveTextContent("none");
    expect(getMeMock).not.toHaveBeenCalled();
  });

  it("Login成功時にAccess TokenとUserをAuthContextへ保存する", async () => {
    refreshAccessTokenMock.mockRejectedValue(new Error("unauthorized"));
    requestLoginMock.mockResolvedValue({
      data: {
        access_token: "login-token",
        token_type: "Bearer",
        expires_in: 900,
        user: {
          id: 1,
          name: "コーヒー太郎",
          email: "coffee@example.com",
          role: "user",
          status: "active",
        },
      },
    });

    renderAuthProvider();

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });

    await userEvent.setup().click(screen.getByRole("button", { name: "Login" }));

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    expect(requestLoginMock).toHaveBeenCalledWith({
      email: "coffee@example.com",
      password: "password123",
    });
    expect(screen.getByTestId("token")).toHaveTextContent("login-token");
    expect(screen.getByTestId("user")).toHaveTextContent(
      "coffee@example.com",
    );
  });

  it("Logout成功時に認証状態を初期化する", async () => {
    refreshAccessTokenMock.mockResolvedValue("restored-token");
    getMeMock.mockResolvedValue({
      data: {
        id: 1,
        name: "コーヒー太郎",
        email: "coffee@example.com",
        role: "user",
        status: "active",
        created_at: "2026-07-21T00:00:00Z",
      },
    });
    requestLogoutMock.mockResolvedValue(undefined);

    renderAuthProvider();

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    await userEvent.setup().click(screen.getByRole("button", { name: "Logout" }));

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("false");
    });

    expect(requestLogoutMock).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("token")).toHaveTextContent("none");
    expect(screen.getByTestId("user")).toHaveTextContent("none");
  });
});
