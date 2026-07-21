import { Link, useNavigate } from "react-router";
import { ApiClientError } from "../api/client";
import { signUp } from "../api/user";
import { useState, type FormEvent } from "react";

const maxNameLength = 50;
const maxEmailLength = 254;
const minPasswordLength = 8;
const maxPasswordBytes = 72;

const inputClass =
  "mt-2 w-full rounded-2xl border border-stone-300/90 bg-white/80 px-4 py-3.5 text-[15px] text-stone-950 outline-none transition placeholder:text-stone-400 hover:border-stone-400 focus:border-amber-700 focus:ring-4 focus:ring-amber-700/10 disabled:cursor-not-allowed disabled:bg-stone-100 disabled:text-stone-500";

function countCharacters(value: string): number {
  return Array.from(value).length;
}

function validateInput(name: string, email: string, password: string): string {
  const normalizedName = name.trim();
  const normalizedEmail = email.trim();

  const nameLength = countCharacters(normalizedName);
  if (nameLength < 1 || nameLength > maxNameLength) {
    return "名前は1文字以上50文字以内で入力してください";
  }

  if (
    normalizedEmail === "" ||
    countCharacters(normalizedEmail) > maxEmailLength
  ) {
    return "メールアドレスを254文字以内で入力してください";
  }

  if (countCharacters(password) < minPasswordLength) {
    return "パスワードは8文字以上で入力してください";
  }

  if (new TextEncoder().encode(password).length > maxPasswordBytes) {
    return "パスワードはUTF-8で72バイト以内にしてください";
  }

  return "";
}

export default function SignupPage() {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState("");
  const [requestId, setRequestId] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (isSubmitting) {
      return;
    }

    setErrorMessage("");
    setRequestId("");

    const validationMessage = validateInput(name, email, password);
    if (validationMessage !== "") {
      setErrorMessage(validationMessage);
      return;
    }

    setIsSubmitting(true);

    try {
      await signUp({
        name,
        email,
        password,
      });

      setPassword("");
      navigate("/login", {
        replace: true,
        state: { registrationCompleted: true },
      });
    } catch (error: unknown) {
      if (error instanceof ApiClientError) {
        setErrorMessage(error.message);
        setRequestId(error.requestId);
      } else {
        setErrorMessage("会員登録に失敗しました");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <>
      <main className="relative min-h-screen overflow-hidden bg-[#100b08] text-stone-100 selection:bg-amber-300 selection:text-stone-950">
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

        <div className="relative mx-auto grid min-h-screen w-full max-w-7xl items-center gap-10 px-4 py-8 sm:px-6 lg:grid-cols-[minmax(0,1fr)_minmax(400px,520px)] lg:px-10 lg:py-12">
          <section
            className="mx-auto w-full max-w-2xl lg:mx-0"
            aria-label="Coffee Reelの紹介"
          >
            <div className="inline-flex items-center gap-3 rounded-full border border-white/10 bg-white/[0.05] px-4 py-2 backdrop-blur-sm">
              <span className="grid h-8 w-8 place-items-center rounded-full bg-amber-300 text-stone-950 shadow-lg shadow-amber-500/20">
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
              <span className="text-sm font-black tracking-[0.18em] text-amber-100 uppercase">
                Coffee Reel
              </span>
            </div>

            <p className="mt-10 text-xs font-bold tracking-[0.3em] text-amber-300 uppercase sm:text-sm">
              Brew. Watch. Share.
            </p>
            <h1 className="mt-4 max-w-xl text-4xl leading-[1.08] font-black tracking-[-0.04em] text-amber-300 sm:text-5xl lg:text-6xl">
              <span className="block text-amber-300">
                一杯の発見を、短い動画から。
              </span>
            </h1>
            <p className="mt-6 max-w-xl text-sm leading-7 text-stone-300 sm:text-base sm:leading-8">
              抽出、焙煎、ラテアート。コーヒーの知識と技術を、縦型ショート動画で見つけて共有するサービスです。
            </p>

            <div className="mt-8 grid gap-3 sm:grid-cols-3">
              {[
                ["01", "見つける", "興味のある動画を探す"],
                ["02", "残す", "気になる動画を保存する"],
                ["03", "共有する", "自分の技術を投稿する"],
              ].map(([number, title, description]) => (
                <div
                  key={number}
                  className="rounded-3xl border border-white/10 bg-white/[0.045] p-4 backdrop-blur-sm"
                >
                  <span className="text-xs font-black tracking-[0.2em] text-amber-300">
                    {number}
                  </span>
                  <p className="mt-3 text-sm font-black text-white">{title}</p>
                  <p className="mt-1 text-xs leading-5 text-stone-400">
                    {description}
                  </p>
                </div>
              ))}
            </div>
          </section>

          <section
            className="relative mx-auto w-full max-w-[520px] rounded-[2.25rem] border border-white/70 bg-[#f7f0e7] p-5 text-[#2a1a12] shadow-[0_32px_90px_rgba(0,0,0,0.38)] sm:p-8 lg:p-10"
            aria-labelledby="signup-title"
          >
            <div
              aria-hidden="true"
              className="absolute top-0 right-10 h-1 w-24 rounded-b-full bg-amber-500"
            />

            <div className="flex items-start justify-between gap-5">
              <div>
                <p className="text-xs font-black tracking-[0.22em] text-amber-800 uppercase">
                  Create account
                </p>
                <h2
                  id="signup-title"
                  className="mt-3 text-3xl font-black tracking-[-0.03em] text-stone-950 sm:text-4xl"
                >
                  会員登録
                </h2>
              </div>
              <span className="rounded-full border border-stone-300 bg-white/70 px-3 py-1.5 text-xs font-bold text-stone-600">
                無料
              </span>
            </div>

            <p className="mt-4 text-sm leading-7 text-stone-600">
              アカウントを作成すると、動画投稿・保存・いいねを利用できます。
            </p>

            <form
              className="mt-8 space-y-5"
              onSubmit={handleSubmit}
              noValidate
              aria-busy={isSubmitting}
            >
              <div>
                <label
                  htmlFor="name"
                  className="text-sm font-black text-stone-800"
                >
                  名前
                </label>
                <input
                  id="name"
                  name="name"
                  type="text"
                  autoComplete="name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  maxLength={maxNameLength}
                  placeholder="コーヒー太郎"
                  className={inputClass}
                  required
                  disabled={isSubmitting}
                />
              </div>

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
                <div className="flex items-end justify-between gap-4">
                  <label
                    htmlFor="password"
                    className="text-sm font-black text-stone-800"
                  >
                    パスワード
                  </label>
                  <span className="text-xs font-medium text-stone-500">
                    8文字以上
                  </span>
                </div>
                <input
                  id="password"
                  name="password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  aria-describedby="password-help signup-error"
                  placeholder="安全なパスワードを入力"
                  className={inputClass}
                  required
                  disabled={isSubmitting}
                />
              </div>

              {errorMessage !== "" && (
                <div
                  id="signup-error"
                  className="rounded-2xl border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
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
                      {requestId !== "" && (
                        <p className="mt-1 text-xs text-red-700">
                          お問い合わせID: {requestId}
                        </p>
                      )}
                    </div>
                  </div>
                </div>
              )}

              <button
                type="submit"
                className="group flex min-h-13 w-full items-center justify-center gap-2 rounded-2xl bg-stone-950 px-5 py-3.5 text-sm font-black text-white shadow-lg shadow-stone-950/20 transition hover:-translate-y-0.5 hover:bg-amber-600 hover:shadow-amber-700/20 focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-700 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:translate-y-0 disabled:hover:bg-stone-950"
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
                    登録中...
                  </>
                ) : (
                  <>
                    登録する
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

            <div className="mt-6 flex items-center gap-3 text-xs text-stone-500">
              <span className="h-px flex-1 bg-stone-300" />
              <span>既に登録済みの方</span>
              <span className="h-px flex-1 bg-stone-300" />
            </div>

            <Link
              to="/login"
              className="mt-4 flex min-h-12 w-full items-center justify-center rounded-2xl border border-stone-300 bg-white/60 px-5 py-3 text-sm font-black text-stone-800 transition hover:border-amber-700 hover:bg-white focus-visible:outline-3 focus-visible:outline-offset-3 focus-visible:outline-amber-700"
            >
              ログインへ進む
            </Link>

            <p className="mt-6 text-center text-[11px] leading-5 text-stone-500">
              パスワードはBackendでハッシュ化され、平文では保存されません。
            </p>
          </section>
        </div>
      </main>
    </>
  );
}
