import { Link, Navigate, Route, Routes, useLocation } from "react-router";
import SignupPage from "../pages/SignupPage";

function LoginDestination() {
  const location = useLocation();
  const state = location.state as { registrationCompleted?: boolean } | null;

  return (
    <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 py-8 text-stone-100">
      <section
        className="w-full max-w-md rounded-[2rem] border border-white/10 bg-white/[0.05] p-7 shadow-2xl shadow-black/40 backdrop-blur-sm sm:p-9"
        aria-labelledby="login-title"
      >
        <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
          Coffee Reel
        </p>
        <h1 id="login-title" className="mt-4 text-3xl font-black text-white">
          ログイン
        </h1>
        <p className="mt-4 text-sm leading-7 text-stone-300">
          {state?.registrationCompleted === true
            ? "会員登録が完了しました。。"
            : "ログイン機能は次の実装手順で接続します。"}
        </p>
        <Link
          to="/signup"
          className="mt-7 flex min-h-12 items-center justify-center rounded-2xl bg-amber-300 px-5 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200 focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-300"
        >
          会員登録へ戻る
        </Link>
      </section>
    </main>
  );
}

function AppRouter() {
  return (
    <Routes>
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/login" element={<LoginDestination />} />
      <Route path="/" element={<Navigate to="/signup" replace />} />
      <Route path="*" element={<Navigate to="/signup" replace />} />
    </Routes>
  );
}

export default AppRouter;
