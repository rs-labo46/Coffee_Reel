import { useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router";

import { ApiClientError } from "../api/client";
import { useAuth } from "../auth/AuthContext";

const maxEmailLength = 254;

const inputClass =
  "mt-2 w-full rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-3.5 text-[15px] text-white outline-none transition placeholder:text-stone-500 hover:border-white/20 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60";

type LoginLocationState = {
  registrationCompleted?: boolean;
};

function validateInput(email: string, password: string): string {
  const normalizedEmail = email.trim();

  if (normalizedEmail === "" || normalizedEmail.length > maxEmailLength) {
    return "メールアドレスを入力してください";
  }

  if (password === "") {
    return "パスワードを入力してください";
  }

  return "";
}

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();
  const locationState = location.state as LoginLocationState | null;

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(
    event: FormEvent<HTMLFormElement>,
  ): Promise<void> {
    event.preventDefault();

    if (isSubmitting) {
      return;
    }

    setErrorMessage("");

    const validationMessage = validateInput(email, password);
    if (validationMessage !== "") {
      setErrorMessage(validationMessage);
      return;
    }

    setIsSubmitting(true);

    try {
      await login({
        email,
        password,
      });

      setPassword("");
      navigate("/", { replace: true });
    } catch (error: unknown) {
      if (error instanceof ApiClientError) {
        setErrorMessage(error.message);
      } else {
        setErrorMessage("ログインに失敗しました");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className="relative min-h-screen overflow-hidden bg-[#100b08] px-4 py-8 text-stone-100 selection:bg-amber-300 selection:text-stone-950 sm:px-6">
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[radial-gradient(circle_at_78%_18%,rgba(245,158,11,0.17),transparent_26%),radial-gradient(circle_at_15%_82%,rgba(120,53,15,0.25),transparent_30%)]"
      />

      <div className="relative mx-auto grid min-h-[calc(100vh-4rem)] w-full max-w-6xl items-center gap-8 lg:grid-cols-[minmax(0,0.9fr)_minmax(420px,1.1fr)]">
        <section className="order-2 rounded-[2.5rem] border border-white/10 bg-white/[0.04] p-7 shadow-2xl shadow-black/30 backdrop-blur-sm sm:p-10 lg:order-1">
          <p className="text-xs font-black tracking-[0.26em] text-amber-300 uppercase">
            Welcome back
          </p>
          <h1 className="mt-5 max-w-md text-4xl leading-tight font-black tracking-[-0.04em] text-white sm:text-5xl">
            今日の一杯に、
            <span className="block text-amber-300">新しい発見を。</span>
          </h1>
          <p className="mt-6 max-w-lg text-sm leading-7 text-stone-300 sm:text-base">
            ログインすると、動画投稿、保存、いいねなどのユーザー機能を利用できます。
          </p>

          <div className="mt-10 grid gap-3 sm:grid-cols-2">
            <div className="rounded-3xl border border-white/10 bg-black/20 p-5">
              <p className="text-xs font-black tracking-[0.2em] text-amber-300 uppercase">
                Watch
              </p>
              <p className="mt-3 text-sm leading-6 text-stone-300">
                気になる抽出や焙煎のショート動画を見つける。
              </p>
            </div>
            <div className="rounded-3xl border border-white/10 bg-black/20 p-5">
              <p className="text-xs font-black tracking-[0.2em] text-amber-300 uppercase">
                Share
              </p>
              <p className="mt-3 text-sm leading-6 text-stone-300">
                自分のコーヒー技術や知識を動画で共有する。
              </p>
            </div>
          </div>
        </section>

        <section
          className="order-1 mx-auto w-full max-w-[520px] rounded-[2.5rem] border border-stone-200 bg-[#f7f0e7] p-6 text-stone-950 shadow-[0_32px_90px_rgba(0,0,0,0.4)] sm:p-9 lg:order-2"
          aria-labelledby="login-title"
        >
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-xs font-black tracking-[0.22em] text-amber-800 uppercase">
                Coffee Reel
              </p>
              <h2
                id="login-title"
                className="mt-3 text-3xl font-black tracking-[-0.03em] text-stone-950 sm:text-4xl"
              >
                ログイン
              </h2>
            </div>
            <span className="grid h-12 w-12 place-items-center rounded-2xl bg-stone-950 text-amber-300">
              <svg
                aria-hidden="true"
                viewBox="0 0 24 24"
                className="h-5 w-5"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <path d="M8.5 5.5c4.8-3.4 10.1-.2 10 4.6-.1 5-5.2 9.2-10.4 8.3-4.6-.8-6.1-6.8-2.6-10.5 1-1.1 2-1.8 3-2.4Z" />
                <path d="M8 17c1.2-3.9 4.2-7.2 8.5-9.4" />
              </svg>
            </span>
          </div>

          {locationState?.registrationCompleted === true && (
            <div
              className="mt-6 rounded-2xl border border-emerald-300 bg-emerald-50 px-4 py-3 text-sm font-bold text-emerald-800"
              role="status"
            >
              会員登録が完了しました。登録した情報でログインしてください。
            </div>
          )}

          <form
            className="mt-7 space-y-5"
            onSubmit={handleSubmit}
            noValidate
            aria-busy={isSubmitting}
          >
            <div>
              <label
                htmlFor="email"
                className="text-sm font-black text-stone-800"
              >
                メールアドレス
              </label>
              <input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                maxLength={maxEmailLength}
                placeholder="coffee@example.com"
                className={inputClass}
                required
                disabled={isSubmitting}
              />
            </div>

            <div>
              <label
                htmlFor="password"
                className="text-sm font-black text-stone-800"
              >
                パスワード
              </label>
              <input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                aria-describedby="login-error"
                placeholder="パスワードを入力"
                className={inputClass}
                required
                disabled={isSubmitting}
              />
            </div>

            {errorMessage !== "" && (
              <div
                id="login-error"
                className="rounded-2xl border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
                role="alert"
              >
                <p className="font-bold">{errorMessage}</p>
              </div>
            )}

            <button
              type="submit"
              className="flex min-h-13 w-full items-center justify-center gap-2 rounded-2xl bg-stone-950 px-5 py-3.5 text-sm font-black text-white shadow-lg shadow-stone-950/20 transition hover:-translate-y-0.5 hover:bg-amber-600 focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-700 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:translate-y-0 disabled:hover:bg-stone-950"
              disabled={isSubmitting}
            >
              {isSubmitting ? (
                <>
                  <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                  ログイン中...
                </>
              ) : (
                "ログイン"
              )}
            </button>
          </form>

          <div className="mt-6 flex items-center gap-3 text-xs text-stone-500">
            <span className="h-px flex-1 bg-stone-300" />
            <span>アカウントをお持ちでない方</span>
            <span className="h-px flex-1 bg-stone-300" />
          </div>

          <Link
            to="/signup"
            className="mt-4 flex min-h-12 w-full items-center justify-center rounded-2xl border border-stone-300 bg-white/60 px-5 py-3 text-sm font-black text-stone-800 transition hover:border-amber-700 hover:bg-white focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-700"
          >
            会員登録へ進む
          </Link>
        </section>
      </div>
    </main>
  );
}
