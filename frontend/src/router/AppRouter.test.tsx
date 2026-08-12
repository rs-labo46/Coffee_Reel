import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { useAuth } from "../auth/useAuth";
import AppRouter from "./AppRouter";

vi.mock("../pages/ReelPage", () => ({
  default: () => <p>ReelPage</p>,
}));

vi.mock("../pages/SearchPage", () => ({
  default: () => <p>SearchPage</p>,
}));

vi.mock("../pages/AuthorVideosPage", () => ({
  default: () => <p>AuthorVideosPage</p>,
}));

vi.mock("../pages/VideoDetailPage", () => ({
  default: () => <p>VideoDetailPage</p>,
}));

vi.mock("../pages/VideoUploadPage", () => ({
  default: () => <p>VideoUploadPage</p>,
}));

vi.mock("../pages/MyVideosPage", () => ({
  default: () => <p>MyVideosPage</p>,
}));

vi.mock("../pages/SavedVideosPage", () => ({
  default: () => <p>SavedVideosPage</p>,
}));

vi.mock("../pages/SignupPage", () => ({
  default: () => <p>SignupPage</p>,
}));

vi.mock("../pages/LoginPage", () => ({
  default: () => <p>LoginPage</p>,
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

vi.mock("../pages/AdminVideosPage", () => ({
  default: () => <p>AdminVideosPage</p>,
}));

vi.mock("../pages/AdminVideoDetailPage", () => ({
  default: () => <p>AdminVideoDetailPage</p>,
}));

vi.mock("../auth/useAuth", () => ({
  useAuth: vi.fn(),
}));

const useAuthMock = vi.mocked(useAuth);

type RouteRole = "guest" | "user" | "admin";

// 指定した認証状態とURLでRouterを描画
function renderRoute(path: string, role: RouteRole = "guest") {
  const isAuthenticated = role !== "guest";

  useAuthMock.mockReturnValue({
    user:
      role === "guest"
        ? null
        : {
            id: role === "admin" ? 2 : 1,
            name: role === "admin" ? "管理者" : "コーヒー太郎",
            email:
              role === "admin" ? "admin@example.com" : "coffee@example.com",
            role,
            status: "active",
          },
    accessToken:
      role === "guest"
        ? null
        : role === "admin"
          ? "admin-access-token"
          : "access-token",
    isAuthenticated,
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
  it("/へReelPageを接続する", () => {
    renderRoute("/");
    expect(screen.getByText("ReelPage")).toBeInTheDocument();
  });

  it("/reelsへReelPageを接続する", () => {
    renderRoute("/reels");
    expect(screen.getByText("ReelPage")).toBeInTheDocument();
  });

  it("/searchへSearchPageを接続する", () => {
    renderRoute("/search?title=drip");
    expect(screen.getByText("SearchPage")).toBeInTheDocument();
  });

  it("/videos/author/:author_idへAuthorVideosPageを接続する", () => {
    renderRoute("/videos/author/10");
    expect(screen.getByText("AuthorVideosPage")).toBeInTheDocument();
  });

  it("/videos/:video_idへVideoDetailPageを接続する", () => {
    renderRoute("/videos/10");
    expect(screen.getByText("VideoDetailPage")).toBeInTheDocument();
  });

  it("/signupへSignupPageを接続する", () => {
    renderRoute("/signup");
    expect(screen.getByText("SignupPage")).toBeInTheDocument();
  });

  it("/loginへLoginPageを接続する", () => {
    renderRoute("/login");
    expect(screen.getByText("LoginPage")).toBeInTheDocument();
  });

  it("/videos/uploadへProtectedRoute内のVideoUploadPageを接続する", () => {
    renderRoute("/videos/upload", "user");
    expect(screen.getByText("VideoUploadPage")).toBeInTheDocument();
  });

  it("/me/videosへProtectedRoute内のMyVideosPageを接続する", () => {
    renderRoute("/me/videos", "user");
    expect(screen.getByText("MyVideosPage")).toBeInTheDocument();
  });

  it("/me/saved-videosへProtectedRoute内のSavedVideosPageを接続する", () => {
    renderRoute("/me/saved-videos", "user");
    expect(screen.getByText("SavedVideosPage")).toBeInTheDocument();
  });

  it("/admin/usersへAdminUsersPageを接続する", () => {
    renderRoute("/admin/users", "admin");
    expect(screen.getByText("AdminUsersPage")).toBeInTheDocument();
  });

  it("/admin/users/:user_idへAdminUserDetailPageを接続する", () => {
    renderRoute("/admin/users/10", "admin");
    expect(screen.getByText("AdminUserDetailPage")).toBeInTheDocument();
  });

  it("/admin/videosへAdminVideosPageを接続する", () => {
    renderRoute("/admin/videos", "admin");
    expect(screen.getByText("AdminVideosPage")).toBeInTheDocument();
  });

  it("/admin/videos/:video_idへAdminVideoDetailPageを接続する", () => {
    renderRoute("/admin/videos/10", "admin");
    expect(screen.getByText("AdminVideoDetailPage")).toBeInTheDocument();
  });

  it("一般ユーザーは管理者投稿画面を表示できない", () => {
    renderRoute("/admin/videos", "user");
    expect(screen.getByText("ReelPage")).toBeInTheDocument();
    expect(screen.queryByText("AdminVideosPage")).not.toBeInTheDocument();
  });

  it("存在しないURLへNotFoundPageを接続する", () => {
    renderRoute("/missing");
    expect(screen.getByText("NotFoundPage")).toBeInTheDocument();
  });
});
