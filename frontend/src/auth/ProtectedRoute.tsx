import { Navigate, Outlet, useLocation } from "react-router";

import { useAuth } from "./AuthContext";

//未認証の時にloginへ移動
export default function ProtectedRoute() {
  const location = useLocation();
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return (
      <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 text-stone-100">
        <div
          className="flex items-center gap-3 text-sm font-bold text-stone-300"
          role="status"
        >
          <span className="h-5 w-5 animate-spin rounded-full border-2 border-amber-300 border-t-transparent" />
          認証状態を確認しています
        </div>
      </main>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }

  return <Outlet />;
}
