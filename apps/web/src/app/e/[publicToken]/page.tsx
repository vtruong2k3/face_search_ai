import { QRCodeSVG } from "qrcode.react";
import * as api from "@/lib/api";
import { SelfieSearch } from "./selfie-search";

export default async function PublicEventPage({ params }: { params: Promise<{ publicToken: string }> }) {
  const { publicToken } = await params;
  let event: api.PublicEvent;
  try {
    event = await api.getPublicEvent(publicToken);
  } catch {
    return <main className="flex min-h-screen items-center justify-center bg-slate-950 px-6 text-slate-100"><section className="max-w-lg text-center"><h1 className="text-3xl font-semibold">Sự kiện không khả dụng</h1><p className="mt-4 text-slate-400">Liên kết không tồn tại, đã hết hạn hoặc không còn công khai.</p></section></main>;
  }
  const origin = process.env.NEXT_PUBLIC_WEB_ORIGIN ?? "http://localhost:3000";
  const canonicalURL = new URL(`/e/${encodeURIComponent(publicToken)}`, origin).toString();
  return <main className="min-h-screen bg-slate-950 px-6 py-16 text-slate-100"><section className="mx-auto max-w-xl rounded-2xl border border-slate-800 bg-slate-900 p-8 text-center"><p className="text-sm font-semibold tracking-[0.2em] text-cyan-400 uppercase">Face Search AI</p><h1 className="mt-4 text-3xl font-semibold">{event.name}</h1><p className="mt-4 text-slate-400">{event.expiresAt ? `Khả dụng đến ${new Date(event.expiresAt).toLocaleString("vi-VN")}` : "Không giới hạn thời gian"}</p><div className="mx-auto mt-8 w-fit rounded-xl bg-white p-4"><QRCodeSVG value={canonicalURL} size={192} title="Mã QR sự kiện" /></div><a className="mt-6 block break-all text-sm text-cyan-300" href={canonicalURL}>{canonicalURL}</a><p className="mt-6 text-sm text-slate-400">{event.downloadsEnabled ? "Sự kiện cho phép tải ảnh." : "Tải ảnh đang tắt."}</p><SelfieSearch publicToken={publicToken} downloadsEnabled={event.downloadsEnabled} /></section></main>;
}
