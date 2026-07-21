import { Link } from "react-router";

export default function NotFoundPage() {
  return (
    <main className="grid min-h-screen place-items-center bg-[#100b08] px-4 py-8 text-stone-100">
      <section className="w-full max-w-md rounded-[2rem] border border-white/10 bg-white/[0.05] p-8 text-center shadow-2xl shadow-black/30">
        <p className="text-xs font-black tracking-[0.24em] text-amber-300 uppercase">
          404
        </p>
        <h1 className="mt-4 text-3xl font-black text-white">
          ページが見つかりません
        </h1>
        <p className="mt-4 text-sm leading-7 text-stone-300">
          URLを確認するか、ログイン画面へ戻ってください。
        </p>
        <Link
          to="/login"
          className="mt-7 flex min-h-12 items-center justify-center rounded-2xl bg-amber-300 px-5 py-3 text-sm font-black text-stone-950 transition hover:bg-amber-200"
        >
          ログイン画面へ戻る
        </Link>
      </section>
    </main>
  );
}
