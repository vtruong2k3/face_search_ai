"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";

export default function EventsPage() {
  const auth = useAuth();
  const router = useRouter();
  const organizationId = auth.currentOrganization?.organizationId;
  const events = useQuery({
    queryKey: ["events", organizationId],
    queryFn: () => api.listEvents(organizationId!),
    enabled: !auth.restoring && Boolean(auth.user && organizationId),
  });

  if (auth.restoring || auth.organizationsLoading) return <Shell><p>Đang tải sự kiện…</p></Shell>;
  if (!auth.user) return <Shell><p>Phiên đăng nhập không hợp lệ.</p><button className="mt-4 text-cyan-300" onClick={() => router.push("/login")}>Đi đến đăng nhập</button></Shell>;
  if (!auth.currentOrganization) return <Shell><p>Bạn cần một tổ chức đang hoạt động để quản lý sự kiện.</p></Shell>;

  return <Shell>
    <div className="flex items-center justify-between gap-4">
      <div><p className="text-sm font-semibold tracking-[0.2em] text-cyan-400 uppercase">{auth.currentOrganization.organizationName}</p><h1 className="mt-2 text-3xl font-semibold">Sự kiện</h1></div>
      {auth.currentOrganization.role !== "viewer" && <Link className="rounded-lg bg-cyan-400 px-4 py-3 font-semibold text-slate-950" href="/events/new">Tạo sự kiện</Link>}
    </div>
    {events.isLoading && <p className="mt-8 text-slate-400">Đang tải sự kiện…</p>}
    {events.isError && <p role="alert" className="mt-8 text-rose-300">Không thể tải danh sách sự kiện.</p>}
    {events.data?.length === 0 && <p className="mt-8 rounded-xl border border-dashed border-slate-700 p-8 text-slate-400">Chưa có sự kiện đang hoạt động.</p>}
    <ul className="mt-8 grid gap-4">
      {events.data?.map((event) => <li key={event.id}><Link className="block rounded-xl border border-slate-800 bg-slate-900 p-5 hover:border-cyan-700" href={`/events/${event.id}`}><h2 className="text-lg font-semibold">{event.name}</h2><p className="mt-2 text-sm text-slate-400">{event.visibility === "public" ? "Công khai" : "Riêng tư"}</p></Link></li>)}
    </ul>
  </Shell>;
}

export function Shell({ children }: { children: React.ReactNode }) {
  return <main className="min-h-screen bg-slate-950 px-6 py-16 text-slate-100"><section className="mx-auto max-w-4xl">{children}</section></main>;
}
