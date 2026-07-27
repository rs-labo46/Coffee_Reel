import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AdminRoute from "./AdminRoute";
import { useAuth } from "./useAuth";

vi.mock("./useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);
const baseAuth = {
  user: null,
  accessToken: null,
  login: vi.fn(),
  logout: vi.fn(),
};

function renderAdminRoute() {
  render(
    <MemoryRouter initialEntries={["/admin/users"]}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="/admin/users" element={<p>管理者ユーザー画面</p>} />
        </Route>
        <Route path="/login" element={<p>ログイン画面</p>} />
        <Route path="/" element={<p>トップ画面</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminRoute", () => {
  beforeEach(() => {
    useAuthMock.mockReset();
  });

  it("認証確認中は管理者権限確認のLoadingだけを表示する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      isAuthenticated: false,
      isLoading: true,
    });

    renderAdminRoute();

    expect(screen.getByRole("status")).toHaveTextContent(
      "管理者権限を確認しています",
    );
    expect(screen.queryByText("管理者ユーザー画面")).not.toBeInTheDocument();
    expect(screen.queryByText("ログイン画面")).not.toBeInTheDocument();
    expect(screen.queryByText("トップ画面")).not.toBeInTheDocument();
  });

  it("未認証ならLogin画面へ遷移する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      isAuthenticated: false,
      isLoading: false,
    });

    renderAdminRoute();

    expect(screen.getByText("ログイン画面")).toBeInTheDocument();
    expect(screen.queryByText("管理者ユーザー画面")).not.toBeInTheDocument();
  });

  it("一般ユーザーならトップ画面へ遷移する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      user: {
        id: 1,
        name: "一般ユーザー",
        email: "user@example.com",
        role: "user",
        status: "active",
      },
      accessToken: "user-access-token",
      isAuthenticated: true,
      isLoading: false,
    });

    renderAdminRoute();

    expect(screen.getByText("トップ画面")).toBeInTheDocument();
    expect(screen.queryByText("管理者ユーザー画面")).not.toBeInTheDocument();
  });

  it("管理者なら管理者ユーザー画面を表示する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      user: {
        id: 2,
        name: "管理者",
        email: "admin@example.com",
        role: "admin",
        status: "active",
      },
      accessToken: "admin-access-token",
      isAuthenticated: true,
      isLoading: false,
    });

    renderAdminRoute();

    expect(screen.getByText("管理者ユーザー画面")).toBeInTheDocument();
    expect(screen.queryByText("ログイン画面")).not.toBeInTheDocument();
    expect(screen.queryByText("トップ画面")).not.toBeInTheDocument();
  });
});
