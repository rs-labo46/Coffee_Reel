import { useState } from "react";

import { ApiClientError } from "../api/client";
import { useAuth } from "../auth/useAuth";

export default function TemporaryHomePage() {
  const { logout, user } = useAuth();
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [isLoggingOut, setIsLoggingOut] = useState<boolean>(false);

  if (user === null) {
    return null;
  }

  async function handleLogout(): Promise<void> {
    if (isLoggingOut) {
      return;
    }

    setErrorMessage("");
    setIsLoggingOut(true);

    try {
      await logout();
    } catch (error: unknown) {
      if (error instanceof ApiClientError) {
        setErrorMessage(error.message);
      } else {
        setErrorMessage("ログアウトに失敗しました");
      }

      setIsLoggingOut(false);
    }
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-10 text-stone-100 sm:px-6">
      <div className="mx-auto w-full max-w-4xl">
        <header className="flex items-center justify-between gap-4">
          <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
            Coffee Reel
          </p>

          <div className="flex items-center gap-3">
            <span className="rounded-full border border-white/10 bg-white/[0.05] px-3 py-1.5 text-xs font-bold text-stone-300">
              認証済み
            </span>

            <button
              type="button"
              onClick={handleLogout}
              disabled={isLoggingOut}
              className="rounded-full border border-amber-300/40 px-4 py-2 text-xs font-black text-amber-200 transition hover:border-amber-300 hover:bg-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {isLoggingOut ? "ログアウト中" : "ログアウト"}
            </button>
          </div>
        </header>

        {errorMessage !== "" && (
          <div
            className="mt-5 rounded-2xl border border-red-400/40 bg-red-950/40 px-4 py-3 text-sm text-red-100"
            role="alert"
          >
            {errorMessage}
          </div>
        )}

        <section className="mt-12 rounded-[2.5rem] border border-white/10 bg-white/[0.05] p-7 shadow-2xl shadow-black/30 sm:p-10">
          <p className="text-sm font-bold text-amber-300">
            ログインに成功しました
          </p>
          <p className="mt-6 text-2xl font-black tracking-[-0.04em] text-white sm:text-5xl">
            {user.name}さん、ようこそ。
          </p>

          <dl className="mt-8 grid gap-4 sm:grid-cols-3">
            <div className="rounded-3xl border border-white/10 bg-black/20 p-5">
              <dt className="text-xs font-bold tracking-[0.16em] text-stone-400 uppercase">
                Name
              </dt>
              <dd className="mt-2 break-words text-base font-black text-white">
                {user.name}
              </dd>
            </div>
            <div className="rounded-3xl border border-white/10 bg-black/20 p-5">
              <dt className="text-xs font-bold tracking-[0.16em] text-stone-400 uppercase">
                Email
              </dt>
              <dd className="mt-2 break-words text-base font-black text-white">
                {user.email}
              </dd>
            </div>
            <div className="rounded-3xl border border-white/10 bg-black/20 p-5">
              <dt className="text-xs font-bold tracking-[0.16em] text-stone-400 uppercase">
                Role
              </dt>
              <dd className="mt-2 text-base font-black text-white">
                {user.role}
              </dd>
            </div>
          </dl>
        </section>
      </div>
    </main>
  );
}
