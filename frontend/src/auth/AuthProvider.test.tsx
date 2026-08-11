import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { refreshAccessToken } from "../api/client";
import {
  fetchCSRFToken,
  getMe,
  login as requestLogin,
  logout as requestLogout,
} from "../api/user";
import {
  clearAuthTokens,
  getAccessToken,
  getCSRFToken,
} from "./tokenStore";
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
  fetchCSRFToken: vi.fn(),
  getMe: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

const refreshAccessTokenMock = vi.mocked(refreshAccessToken);
const fetchCSRFTokenMock = vi.mocked(fetchCSRFToken);
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
    clearAuthTokens();
    refreshAccessTokenMock.mockReset();
    fetchCSRFTokenMock.mockReset();
    getMeMock.mockReset();
    requestLoginMock.mockReset();
    requestLogoutMock.mockReset();
  });

  it("ページ読込時にCSRF取得、Refresh、Meの順で認証状態を復元する", async () => {
    const callOrder: string[] = [];

    fetchCSRFTokenMock.mockImplementation(async () => {
      callOrder.push("csrf");
      return {
        data: {
          csrf_token: "bootstrap-csrf",
        },
      };
    });
    refreshAccessTokenMock.mockImplementation(async () => {
      callOrder.push("refresh");
      expect(getCSRFToken()).toBe("bootstrap-csrf");
      return "restored-token";
    });
    getMeMock.mockImplementation(async () => {
      callOrder.push("me");
      return {
        data: {
          id: 1,
          name: "コーヒー太郎",
          email: "coffee@example.com",
          role: "user",
          status: "active",
          created_at: "2026-07-21T00:00:00Z",
        },
      };
    });

    renderAuthProvider();

    expect(screen.getByTestId("loading")).toHaveTextContent("true");

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    expect(callOrder).toEqual(["csrf", "refresh", "me"]);
    expect(fetchCSRFTokenMock).toHaveBeenCalledTimes(1);
    expect(refreshAccessTokenMock).toHaveBeenCalledTimes(1);
    expect(getMeMock).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("token")).toHaveTextContent("restored-token");
    expect(screen.getByTestId("user")).toHaveTextContent("coffee@example.com");
  });

  it("CSRF取得または認証復元に失敗した場合は未認証状態へ初期化する", async () => {
    fetchCSRFTokenMock.mockRejectedValue(new Error("network error"));

    renderAuthProvider();

    await waitFor(() => {
      expect(screen.getByTestId("loading")).toHaveTextContent("false");
    });

    expect(screen.getByTestId("authenticated")).toHaveTextContent("false");
    expect(screen.getByTestId("token")).toHaveTextContent("none");
    expect(screen.getByTestId("user")).toHaveTextContent("none");
    expect(refreshAccessTokenMock).not.toHaveBeenCalled();
    expect(getMeMock).not.toHaveBeenCalled();
    expect(getCSRFToken()).toBeNull();
  });

  it("Login成功時にAccess Token、CSRF Token、Userを保存する", async () => {
    fetchCSRFTokenMock.mockRejectedValue(new Error("unauthorized"));
    requestLoginMock.mockResolvedValue({
      data: {
        access_token: "login-token",
        token_type: "Bearer",
        expires_in: 900,
        csrf_token: "login-csrf",
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
    expect(screen.getByTestId("user")).toHaveTextContent("coffee@example.com");
    expect(getAccessToken()).toBe("login-token");
    expect(getCSRFToken()).toBe("login-csrf");
  });

  it("Logout成功時にAccess TokenとCSRF Tokenを両方削除する", async () => {
    fetchCSRFTokenMock.mockResolvedValue({
      data: {
        csrf_token: "bootstrap-csrf",
      },
    });
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

    expect(getCSRFToken()).toBe("bootstrap-csrf");

    await userEvent.setup().click(screen.getByRole("button", { name: "Logout" }));

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("false");
    });

    expect(requestLogoutMock).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("token")).toHaveTextContent("none");
    expect(screen.getByTestId("user")).toHaveTextContent("none");
    expect(getAccessToken()).toBeNull();
    expect(getCSRFToken()).toBeNull();
  });
});
