import { Route, Routes } from "react-router";
import SignupPage from "../pages/SignupPage";
import LoginPage from "../pages/LoginPage";
import ProtectedRoute from "../auth/ProtectedRoute";
import NotFoundPage from "../pages/NotFoundPage";
import AdminRoute from "../auth/AdminRoute";
import AdminUsersPage from "../pages/AdminUsersPage";
import AdminUserDetailPage from "../pages/AdminUserDetailPage";

import VideoUploadPage from "../pages/VideoUploadPage";
import MyVideosPage from "../pages/MyVideosPage";
import SavedVideosPage from "../pages/SavedVideosPage";
import VideoDetailPage from "../pages/VideoDetailPage";
import ReelPage from "../pages/ReelPage";
import AdminVideosPage from "../pages/AdminVideosPage";
import AdminVideoDetailPage from "../pages/AdminVideoDetailPage";

export default function AppRouter() {
  return (
    <Routes>
      <Route path="/" element={<ReelPage />} />
      <Route path="/reels" element={<ReelPage />} />
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
