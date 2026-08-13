import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { refreshSession } from "../api/client";
import {
  fetchCSRFToken,
  login as requestLogin,
  logout as requestLogout,
} from "../api/user";
import {
  clearAuthTokens,
  getAccessToken,
  getCSRFToken,
  setAccessToken,
  setCSRFToken,
} from "./tokenStore";
import { AuthProvider } from "./AuthProvider";
import { useAuth } from "./useAuth";

vi.mock("../api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/client")>();

  return {
    ...actual,
    refreshSession: vi.fn(),
  };
});

vi.mock("../api/user", () => ({
  fetchCSRFToken: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

const refreshSessionMock = vi.mocked(refreshSession);
const fetchCSRFTokenMock = vi.mocked(fetchCSRFToken);
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
    refreshSessionMock.mockReset();
    fetchCSRFTokenMock.mockReset();
    requestLoginMock.mockReset();
    requestLogoutMock.mockReset();
  });

  it("ページ読込時にCSRF取得後のRefresh Responseだけで認証状態を復元する", async () => {
    const callOrder: string[] = [];

    fetchCSRFTokenMock.mockImplementation(async () => {
      callOrder.push("csrf");

      return {
        data: {
          csrf_token: "bootstrap-csrf",
        },
      };
    });

    refreshSessionMock.mockImplementation(async () => {
      callOrder.push("refresh");

      expect(getCSRFToken()).toBe("bootstrap-csrf");

      // ---追加---
      // 実際のrefreshSession()はRefresh成功時に
      // Access Tokenと新しいCSRF TokenをToken Storeへ保存する。
      // Mockでも同じ副作用を再現する。
      setAccessToken("restored-token");
      setCSRFToken("rotated-csrf");
      // ---追加---

      return {
        access_token: "restored-token",
        token_type: "Bearer",
        expires_in: 900,
        csrf_token: "rotated-csrf",
        user: {
          id: 1,
          name: "コーヒー太郎",
          email: "coffee@example.com",
          role: "user",
          status: "active",
        },
      };
    });

    renderAuthProvider();

    expect(screen.getByTestId("loading")).toHaveTextContent("true");

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    expect(callOrder).toEqual(["csrf", "refresh"]);
    expect(fetchCSRFTokenMock).toHaveBeenCalledTimes(1);
    expect(refreshSessionMock).toHaveBeenCalledTimes(1);

    expect(screen.getByTestId("token")).toHaveTextContent("restored-token");
    expect(screen.getByTestId("user")).toHaveTextContent("coffee@example.com");

    // ---追加---
    expect(getAccessToken()).toBe("restored-token");
    // ---追加---

    expect(getCSRFToken()).toBe("rotated-csrf");
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
    expect(refreshSessionMock).not.toHaveBeenCalled();
    expect(getAccessToken()).toBeNull();
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

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Login" }));

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

    // ---変更---
    refreshSessionMock.mockImplementation(async () => {
      setAccessToken("restored-token");
      setCSRFToken("rotated-csrf");

      return {
        access_token: "restored-token",
        token_type: "Bearer",
        expires_in: 900,
        csrf_token: "rotated-csrf",
        user: {
          id: 1,
          name: "コーヒー太郎",
          email: "coffee@example.com",
          role: "user",
          status: "active",
        },
      };
    });
    // ---変更---

    requestLogoutMock.mockResolvedValue(undefined);

    renderAuthProvider();

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("true");
    });

    // ---変更---
    // Refresh成功後なのでbootstrap値ではなく
    // Rotation後のCSRF Tokenが保持されている。
    expect(getAccessToken()).toBe("restored-token");
    expect(getCSRFToken()).toBe("rotated-csrf");
    // ---変更---

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Logout" }));

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
