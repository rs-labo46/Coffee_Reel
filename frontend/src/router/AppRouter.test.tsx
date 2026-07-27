import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { useAuth } from "../auth/useAuth";
import AppRouter from "./AppRouter";

vi.mock("../pages/SignupPage", () => ({
  default: () => <p>SignupPage</p>,
}));

vi.mock("../pages/LoginPage", () => ({
  default: () => <p>LoginPage</p>,
}));

vi.mock("../pages/TemporaryHomePage", () => ({
  default: () => <p>TemporaryHomePage</p>,
}));

vi.mock("../pages/NotFoundPage", () => ({
  default: () => <p>NotFoundPage</p>,
}));
vi.mock("../pages/AdminUsersPage", () => ({
  default: () => <p>AdminUsersPage</p>,
}));

vi.mock("../pages/AdminUserDetailPage", () => ({
  default: () => <p>AdminUserDetailPage</p>,
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);

// ------- 変更コード -------
function renderRoute(path: string, role: "user" | "admin" = "user") {
  useAuthMock.mockReturnValue({
    user: {
      id: role === "admin" ? 2 : 1,
      name: role === "admin" ? "管理者" : "コーヒー太郎",
      email: role === "admin" ? "admin@example.com" : "coffee@example.com",
      role,
      status: "active",
    },
    accessToken: role === "admin" ? "admin-access-token" : "access-token",
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
  });

  render(
    <MemoryRouter initialEntries={[path]}>
      <AppRouter />
    </MemoryRouter>,
  );
}
// ------- 変更コードここまで -------

describe("AppRouter", () => {
  it("/signupへSignupPageを接続する", () => {
    renderRoute("/signup");
    expect(screen.getByText("SignupPage")).toBeInTheDocument();
  });

  it("/loginへLoginPageを接続する", () => {
    renderRoute("/login");
    expect(screen.getByText("LoginPage")).toBeInTheDocument();
  });

  it("/へProtectedRoute内のTemporaryHomePageを接続する", () => {
    renderRoute("/");
    expect(screen.getByText("TemporaryHomePage")).toBeInTheDocument();
  });

  it("/admin/usersへAdminUsersPageを接続する", () => {
    renderRoute("/admin/users", "admin");

    expect(screen.getByText("AdminUsersPage")).toBeInTheDocument();
  });

  it("/admin/users/:user_idへAdminUserDetailPageを接続する", () => {
    renderRoute("/admin/users/10", "admin");

    expect(screen.getByText("AdminUserDetailPage")).toBeInTheDocument();
  });

  it("存在しないURLへNotFoundPageを接続する", () => {
    renderRoute("/missing");
    expect(screen.getByText("NotFoundPage")).toBeInTheDocument();
  });
});
