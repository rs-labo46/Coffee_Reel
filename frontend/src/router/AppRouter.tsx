import { Route, Routes } from "react-router";

import AdminRoute from "../auth/AdminRoute";
import ProtectedRoute from "../auth/ProtectedRoute";
import AdminUserDetailPage from "../pages/AdminUserDetailPage";
import AdminUsersPage from "../pages/AdminUsersPage";
import AdminVideoDetailPage from "../pages/AdminVideoDetailPage";
import AdminVideosPage from "../pages/AdminVideosPage";
import LoginPage from "../pages/LoginPage";
import MyVideosPage from "../pages/MyVideosPage";
import NotFoundPage from "../pages/NotFoundPage";
import ReelPage from "../pages/ReelPage";
import SavedVideosPage from "../pages/SavedVideosPage";
import SearchPage from "../pages/SearchPage";
import SignupPage from "../pages/SignupPage";
import VideoDetailPage from "../pages/VideoDetailPage";
import VideoUploadPage from "../pages/VideoUploadPage";

// URL、認証保護、管理者保護、画面Componentを接続
export default function AppRouter() {
  return (
    <Routes>
      <Route path="/" element={<ReelPage />} />
      <Route path="/reels" element={<ReelPage />} />
      <Route path="/search" element={<SearchPage />} />
      <Route path="/videos/:video_id" element={<VideoDetailPage />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/login" element={<LoginPage />} />

      <Route element={<ProtectedRoute />}>
        <Route path="/videos/upload" element={<VideoUploadPage />} />
        <Route path="/me/videos" element={<MyVideosPage />} />
        <Route path="/me/saved-videos" element={<SavedVideosPage />} />
      </Route>

      <Route element={<AdminRoute />}>
        <Route path="/admin/users" element={<AdminUsersPage />} />
        <Route path="/admin/users/:user_id" element={<AdminUserDetailPage />} />
        <Route path="/admin/videos" element={<AdminVideosPage />} />
        <Route
          path="/admin/videos/:video_id"
          element={<AdminVideoDetailPage />}
        />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
