import { useState, type FormEvent } from "react";
import { Link, useLocation, useNavigate } from "react-router";

import { ApiClientError } from "../api/client";
import { useAuth } from "../auth/useAuth";

const maxEmailLength = 254;

const inputClass =
  "mt-2 w-full rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-3.5 text-[15px] text-stone-950 outline-none transition placeholder:text-stone-500 hover:border-white/20 focus:border-amber-300/70 focus:ring-4 focus:ring-amber-300/10 disabled:cursor-not-allowed disabled:opacity-60";

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
function safeRedirectPath(value: string | null): string {
  if (value === null || !value.startsWith("/") || value.startsWith("//")) {
    return "/";
  }

  try {
    const base = "https://coffee-reel.local";
    const parsed = new URL(value, base);

    if (parsed.origin !== base) {
      return "/";
    }

    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return "/";
  }
}

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();
  const locationState = location.state as LoginLocationState | null;
  const redirect = new URLSearchParams(location.search).get("redirect");

  const [email, setEmail] = useState<string>("");
  const [password, setPassword] = useState<string>("");
  const [errorMessage, setErrorMessage] = useState<string>("");
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

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
      navigate(safeRedirectPath(redirect), { replace: true });
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
    <main className="relative min-h-dvh overflow-x-hidden bg-[#100b08] text-stone-100 selection:bg-amber-300 selection:text-stone-950">
      <div
        aria-hidden="true"
        className="absolute inset-0 bg-[radial-gradient(circle_at_12%_18%,rgba(217,119,6,0.22),transparent_28%),radial-gradient(circle_at_82%_74%,rgba(120,53,15,0.28),transparent_32%)]"
      />

      <div
        aria-hidden="true"
        className="absolute -left-24 top-1/3 h-64 w-64 rounded-full border border-amber-300/10"
      />

      <div
        aria-hidden="true"
        className="absolute -right-32 -top-24 h-96 w-96 rounded-full border border-white/5"
      />

      <div className="relative mx-auto grid w-full max-w-7xl content-start gap-5 px-4 py-4 sm:gap-7 sm:px-6 sm:py-7 lg:min-h-dvh lg:grid-cols-[minmax(0,1fr)_minmax(400px,520px)] lg:items-center lg:gap-12 lg:px-10 lg:py-10">
        <section
          className="mx-auto w-full max-w-2xl text-center lg:mx-0 lg:text-left"
          aria-label="Coffee Reelの紹介"
        >
          <div className="inline-flex items-center gap-2.5 rounded-full border border-white/10 bg-white/[0.05] px-3.5 py-1.5 backdrop-blur-sm sm:gap-3 sm:px-4 sm:py-2">
            <span className="grid h-7 w-7 place-items-center rounded-full bg-amber-300 text-stone-950 shadow-lg shadow-amber-500/20 sm:h-8 sm:w-8">
              <svg
                aria-hidden="true"
                viewBox="0 0 24 24"
                className="h-4 w-4"
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

            <span className="text-xs font-black tracking-[0.18em] text-amber-100 uppercase sm:text-sm">
              Coffee Reel
            </span>
          </div>

          <p className="mt-5 text-[11px] font-bold tracking-[0.28em] text-amber-300 uppercase sm:mt-7 sm:text-xs lg:mt-10 lg:text-sm">
            Welcome back
          </p>

          <h1 className="mx-auto mt-2.5 max-w-xl font-black tracking-[-0.04em] text-amber-300 sm:mt-4 lg:mx-0">
            <span className="block whitespace-nowrap text-[clamp(2rem,8vw,3.75rem)] leading-[1.08]">
              今日の一杯に、
            </span>

            <span className="mt-1 block whitespace-nowrap text-[clamp(2rem,8vw,3.75rem)] leading-[1.08]">
              新しい発見を。
            </span>
          </h1>

          <p className="mx-auto mt-3 max-w-xl text-xs leading-6 text-stone-300 sm:mt-5 sm:text-sm sm:leading-7 lg:mx-0 lg:text-base lg:leading-8">
            ログインすると、動画投稿・保存・いいねなどのユーザー機能を利用できます。
          </p>

          <div className="mt-4 grid grid-cols-2 gap-2 sm:mt-6 sm:gap-3 lg:mt-8">
            <div className="min-w-0 rounded-2xl border border-white/10 bg-white/[0.045] px-3 py-3 backdrop-blur-sm sm:rounded-3xl sm:p-5">
              <span className="text-[10px] font-black tracking-[0.2em] text-amber-300 uppercase sm:text-xs">
                Watch
              </span>

              <p className="mt-2 text-xs font-black text-white sm:mt-3 sm:text-sm">
                見つける
              </p>

              <p className="mt-1 hidden text-xs leading-5 text-stone-400 sm:block">
                気になる抽出や焙煎の動画を見る
              </p>
            </div>

            <div className="min-w-0 rounded-2xl border border-white/10 bg-white/[0.045] px-3 py-3 backdrop-blur-sm sm:rounded-3xl sm:p-5">
              <span className="text-[10px] font-black tracking-[0.2em] text-amber-300 uppercase sm:text-xs">
                Share
              </span>

              <p className="mt-2 text-xs font-black text-white sm:mt-3 sm:text-sm">
                共有する
              </p>

              <p className="mt-1 hidden text-xs leading-5 text-stone-400 sm:block">
                自分の技術や知識を動画で共有する
              </p>
            </div>
          </div>
        </section>

        <section
          className="relative mx-auto w-full max-w-[520px] overflow-hidden rounded-[1.75rem] border border-white/20 bg-white/[0.09] p-4 text-left text-stone-100 shadow-[0_28px_80px_rgba(0,0,0,0.44)] ring-1 ring-inset ring-white/10 backdrop-blur-2xl sm:rounded-[2.25rem] sm:p-7 lg:p-9"
          aria-labelledby="login-title"
        >
          <div
            aria-hidden="true"
            className="absolute top-0 right-8 h-1 w-20 rounded-b-full bg-gradient-to-r from-amber-200/80 to-amber-500/80 shadow-[0_0_24px_rgba(251,191,36,0.45)] sm:right-10 sm:w-24"
          />

          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-[10px] font-black tracking-[0.22em] text-amber-200 uppercase sm:text-xs">
                Welcome back
              </p>

              <h2
                id="login-title"
                className="mt-1.5 text-2xl font-black tracking-[-0.03em] text-white drop-shadow-sm sm:mt-3 sm:text-4xl"
              >
                ログイン
              </h2>
            </div>

            <span className="rounded-full border border-white/20 bg-white/10 px-2.5 py-1 text-[11px] font-bold text-stone-200 shadow-sm backdrop-blur-md sm:px-3 sm:py-1.5 sm:text-xs">
              会員
            </span>
          </div>

          <p className="mt-2 text-xs leading-5 text-stone-300 sm:mt-4 sm:text-sm sm:leading-7">
            登録したメールアドレスとパスワードを入力してください。
          </p>

          {locationState?.registrationCompleted === true && (
            <div
              className="mt-4 rounded-xl border border-emerald-300/30 bg-emerald-400/10 px-3.5 py-2.5 text-xs font-bold leading-5 text-emerald-100 shadow-lg shadow-emerald-950/10 backdrop-blur-md sm:rounded-2xl sm:text-sm"
              role="status"
            >
              会員登録が完了しました。登録した情報でログインしてください。
            </div>
          )}

          <form
            className="mt-5 space-y-3.5 sm:mt-7 sm:space-y-5"
            onSubmit={handleSubmit}
            noValidate
            aria-busy={isSubmitting}
          >
            <div>
              <label
                htmlFor="email"
                className="text-xs font-black text-stone-100 sm:text-sm"
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
                className="text-xs font-black text-stone-100 sm:text-sm"
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
                className="rounded-2xl border border-red-300/30 bg-red-400/10 px-4 py-3 text-sm text-red-100 shadow-lg shadow-red-950/10 backdrop-blur-md"
                role="alert"
              >
                <div className="flex gap-3">
                  <svg
                    aria-hidden="true"
                    viewBox="0 0 24 24"
                    className="mt-0.5 h-5 w-5 shrink-0"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <circle cx="12" cy="12" r="9" />
                    <path d="M12 8v5" />
                    <path d="M12 16.5h.01" />
                  </svg>

                  <div>
                    <p className="font-bold">{errorMessage}</p>
                  </div>
                </div>
              </div>
            )}

            <button
              type="submit"
              className="group flex min-h-12 w-full items-center justify-center gap-2 whitespace-nowrap rounded-xl border border-amber-100/30 bg-gradient-to-r from-amber-300/95 to-amber-500/95 px-5 py-3 text-sm font-black text-stone-950 shadow-[0_14px_34px_rgba(217,119,6,0.28)] backdrop-blur-md transition hover:-translate-y-0.5 hover:brightness-105 hover:shadow-[0_18px_42px_rgba(217,119,6,0.34)] focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-300 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:translate-y-0 disabled:hover:brightness-100 sm:min-h-13 sm:rounded-2xl sm:py-3.5"
              disabled={isSubmitting}
            >
              {isSubmitting ? (
                <>
                  <svg
                    aria-hidden="true"
                    viewBox="0 0 24 24"
                    className="h-4 w-4 animate-spin"
                    fill="none"
                  >
                    <circle
                      cx="12"
                      cy="12"
                      r="9"
                      stroke="currentColor"
                      strokeWidth="3"
                      className="opacity-25"
                    />

                    <path
                      d="M21 12a9 9 0 0 0-9-9"
                      stroke="currentColor"
                      strokeWidth="3"
                      strokeLinecap="round"
                    />
                  </svg>
                  ログイン中...
                </>
              ) : (
                <>
                  ログイン
                  <svg
                    aria-hidden="true"
                    viewBox="0 0 24 24"
                    className="h-4 w-4 transition group-hover:translate-x-0.5"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <path d="M5 12h14" />
                    <path d="m14 7 5 5-5 5" />
                  </svg>
                </>
              )}
            </button>
          </form>

          <div className="mt-4 flex items-center gap-3 text-[11px] text-stone-400 sm:mt-6 sm:text-xs">
            <span className="h-px flex-1 bg-white/15" />

            <span className="shrink-0 whitespace-nowrap">
              アカウントをお持ちでない方
            </span>

            <span className="h-px flex-1 bg-white/15" />
          </div>

          <Link
            to="/signup"
            className="mt-3 flex min-h-11 w-full items-center justify-center whitespace-nowrap rounded-xl border border-white/20 bg-white/[0.08] px-5 py-2.5 text-sm font-black text-white shadow-sm backdrop-blur-md transition hover:border-amber-200/50 hover:bg-white/[0.14] focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-300 sm:mt-4 sm:min-h-12 sm:rounded-2xl sm:py-3"
          >
            会員登録へ進む
          </Link>
        </section>
      </div>
    </main>
  );
}
