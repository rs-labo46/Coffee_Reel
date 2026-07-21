import { useAuth } from "../auth/useAuth";

export default function TemporaryHomePage() {
  const { user } = useAuth();

  if (user === null) {
    return null;
  }

  return (
    <main className="min-h-screen bg-[#100b08] px-4 py-10 text-stone-100 sm:px-6">
      <div className="mx-auto w-full max-w-4xl">
        <header className="flex items-center justify-between gap-4">
          <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
            Coffee Reel
          </p>
          <span className="rounded-full border border-white/10 bg-white/[0.05] px-3 py-1.5 text-xs font-bold text-stone-300">
            認証済み
          </span>
        </header>

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
