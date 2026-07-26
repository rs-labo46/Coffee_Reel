import { Route, Routes } from "react-router";
import SignupPage from "../pages/SignupPage";
import LoginPage from "../pages/LoginPage";
import ProtectedRoute from "../auth/ProtectedRoute";
import TemporaryHomePage from "../pages/TemporaryHomePage";
import NotFoundPage from "../pages/NotFoundPage";
import AdminRoute from "../auth/AdminRoute";
import AdminUsersPage from "../pages/AdminUsersPage";
import AdminUserDetailPage from "../pages/AdminUserDetailPage";

export default function AppRouter() {
  return (
    <Routes>
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<ProtectedRoute />}>
        <Route path="/" element={<TemporaryHomePage />} />
      </Route>
      <Route element={<AdminRoute />}>
        <Route path="/admin/users" element={<AdminUsersPage />} />
        <Route path="/admin/users/:user_id" element={<AdminUserDetailPage />} />
      </Route>
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
