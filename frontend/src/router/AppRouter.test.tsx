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

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);

function renderRoute(path: string) {
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
    logout: vi.fn(),
  });

  render(
    <MemoryRouter initialEntries={[path]}>
      <AppRouter />
    </MemoryRouter>,
  );
}

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

  it("存在しないURLへNotFoundPageを接続する", () => {
    renderRoute("/missing");
    expect(screen.getByText("NotFoundPage")).toBeInTheDocument();
  });
});
