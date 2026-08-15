"use client";

import { useRouter } from "next/navigation";
import { useAuth } from "@/components/providers/auth-provider";

export default function AccountPage() {
  const { user, restoring, logout } = useAuth();
  const router = useRouter();

  if (restoring) return <main className="min-h-screen bg-slate-950 p-16 text-slate-100">Đang khôi phục phiên…</main>;
  if (!user) return <main className="min-h-screen bg-slate-950 p-16 text-slate-100"><p>Phiên đăng nhập không hợp lệ.</p><button className="mt-4 text-cyan-300" onClick={() => router.push("/login")}>Đi đến đăng nhập</button></main>;

  return (
    <main className="min-h-screen bg-slate-950 px-6 py-16 text-slate-100">
      <section className="mx-auto max-w-2xl rounded-2xl border border-slate-800 bg-slate-900 p-8">
        <p className="text-sm font-semibold tracking-[0.2em] text-cyan-400 uppercase">Tài khoản</p>
        <h1 className="mt-3 text-3xl font-semibold">{user.email}</h1>
        <dl className="mt-8 grid gap-4 text-sm">
          <div><dt className="text-slate-400">Trạng thái</dt><dd className="mt-1">{user.status}</dd></div>
          <div><dt className="text-slate-400">Mã người dùng</dt><dd className="mt-1 font-mono">{user.id}</dd></div>
        </dl>
        <button className="mt-8 rounded-lg border border-slate-700 px-4 py-3" onClick={async () => { await logout(); router.push("/login"); }}>Đăng xuất</button>
      </section>
    </main>
  );
}
