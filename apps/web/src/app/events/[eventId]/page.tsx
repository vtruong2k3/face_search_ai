"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";
import { Shell } from "../page";
import { PhotoUploader } from "./photo-uploader";

export default function EventDetailPage() {
  const { eventId } = useParams<{ eventId: string }>();
  const auth = useAuth();
  const organization = auth.currentOrganization;
  const detail = useQuery({ queryKey: ["event", organization?.organizationId, eventId], queryFn: () => api.getEvent(organization!.organizationId, eventId), enabled: Boolean(organization && eventId) });
  const status = useQuery({ queryKey: ["event-status", organization?.organizationId, eventId], queryFn: () => api.getEventStatus(organization!.organizationId, eventId), enabled: Boolean(organization && eventId) });

  if (auth.restoring || auth.organizationsLoading || detail.isLoading) return <Shell><p>Đang tải sự kiện…</p></Shell>;
  if (!organization) return <Shell><p>Không có tổ chức đang hoạt động.</p></Shell>;
  if (detail.isError || !detail.data) return <Shell><p role="alert">Sự kiện không tồn tại hoặc không khả dụng.</p></Shell>;
  const event = detail.data;
  return <Shell><div className="flex items-start justify-between gap-4"><div><p className="text-sm text-cyan-400">{event.visibility === "public" ? "Công khai" : "Riêng tư"}</p><h1 className="mt-2 text-3xl font-semibold">{event.name}</h1></div>{organization.role !== "viewer" && <Link className="rounded-lg border border-slate-700 px-4 py-3" href={`/events/${event.id}/settings`}>Cài đặt</Link>}</div>
    <dl className="mt-8 grid gap-4 rounded-xl border border-slate-800 bg-slate-900 p-6 sm:grid-cols-2"><div><dt className="text-slate-400">Hết hạn</dt><dd>{event.expiresAt ? new Date(event.expiresAt).toLocaleString("vi-VN") : "Không giới hạn"}</dd></div><div><dt className="text-slate-400">Tải xuống</dt><dd>{event.downloadsEnabled ? "Cho phép" : "Tắt"}</dd></div></dl>
    <h2 className="mt-8 text-xl font-semibold">Xử lý ảnh</h2>{status.isLoading && <p className="mt-3 text-slate-400">Đang tải trạng thái…</p>}{status.data && <p className="mt-3 text-slate-300">{status.data.ready}/{status.data.activeTotal} ảnh sẵn sàng · {status.data.failed} lỗi</p>}
    {organization.role !== "viewer" && <PhotoUploader organizationId={organization.organizationId} eventId={event.id} />}
  </Shell>;
}
