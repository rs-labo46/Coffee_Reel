import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useAuth } from "./useAuth";
import ProtectedRoute from "./ProtectedRoute";

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

function renderProtectedRoute() {
  render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route path="/" element={<p>保護画面</p>} />
        </Route>
        <Route path="/login" element={<p>ログイン画面</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ProtectedRoute", () => {
  beforeEach(() => {
    useAuthMock.mockReset();
  });

  it("認証確認中はLoadingだけを表示する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      isAuthenticated: false,
      isLoading: true,
    });

    renderProtectedRoute();

    expect(screen.getByRole("status")).toHaveTextContent(
      "認証状態を確認しています",
    );
    expect(screen.queryByText("保護画面")).not.toBeInTheDocument();
    expect(screen.queryByText("ログイン画面")).not.toBeInTheDocument();
  });

  it("認証済みなら保護画面を表示する", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
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
    });

    renderProtectedRoute();

    expect(screen.getByText("保護画面")).toBeInTheDocument();
  });

  it("未認証ならLogin画面へ遷移し、保護画面を表示しない", () => {
    useAuthMock.mockReturnValue({
      ...baseAuth,
      isAuthenticated: false,
      isLoading: false,
    });

    renderProtectedRoute();

    expect(screen.getByText("ログイン画面")).toBeInTheDocument();
    expect(screen.queryByText("保護画面")).not.toBeInTheDocument();
  });
});
