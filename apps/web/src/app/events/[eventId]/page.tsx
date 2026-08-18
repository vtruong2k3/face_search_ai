"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";
import { Shell } from "../page";
import { PhotoUploader } from "./photo-uploader";

const PROCESSING_POLL_MS = 3_000;

export default function EventDetailPage() {
  const { eventId } = useParams<{ eventId: string }>();
  const auth = useAuth();
  const organization = auth.currentOrganization;
  const queryClient = useQueryClient();
  const organizationId = organization?.organizationId;
  const scopeEnabled = Boolean(organizationId && eventId);

  const detail = useQuery({
    queryKey: ["event", organizationId, eventId],
    queryFn: () => api.getEvent(organizationId!, eventId),
    enabled: scopeEnabled,
  });
  const status = useQuery({
    queryKey: ["event-status", organizationId, eventId],
    queryFn: () => api.getEventStatus(organizationId!, eventId),
    enabled: scopeEnabled,
    refetchInterval: (query) => {
      const value = query.state.data;
      if (!value) return PROCESSING_POLL_MS;
      return value.queued + value.processing + value.uploading > 0 ? PROCESSING_POLL_MS : false;
    },
  });
  const photos = useQuery({
    queryKey: ["event-photos", organizationId, eventId],
    queryFn: () => api.listPhotos(organizationId!, eventId),
    enabled: scopeEnabled,
    refetchInterval: status.data?.queued || status.data?.processing ? PROCESSING_POLL_MS : false,
  });
  const retry = useMutation({
    mutationFn: (photoId: string) => api.reprocessPhoto(organizationId!, eventId, photoId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["event-status", organizationId, eventId] }),
        queryClient.invalidateQueries({ queryKey: ["event-photos", organizationId, eventId] }),
      ]);
    },
  });

  if (auth.restoring || auth.organizationsLoading || detail.isLoading) return <Shell><p>Đang tải sự kiện…</p></Shell>;
  if (!organization) return <Shell><p>Không có tổ chức đang hoạt động.</p></Shell>;
  if (detail.isError || !detail.data) return <Shell><p role="alert">Sự kiện không tồn tại hoặc không khả dụng.</p></Shell>;
  const event = detail.data;
  const failedPhotos = photos.data?.filter((photo) => photo.status === "failed") ?? [];

  return <Shell>
    <div className="flex items-start justify-between gap-4">
      <div><p className="text-sm text-cyan-400">{event.visibility === "public" ? "Công khai" : "Riêng tư"}</p><h1 className="mt-2 text-3xl font-semibold">{event.name}</h1></div>
      {organization.role !== "viewer" && <Link className="rounded-lg border border-slate-700 px-4 py-3" href={`/events/${event.id}/settings`}>Cài đặt</Link>}
    </div>
    <dl className="mt-8 grid gap-4 rounded-xl border border-slate-800 bg-slate-900 p-6 sm:grid-cols-2"><div><dt className="text-slate-400">Hết hạn</dt><dd>{event.expiresAt ? new Date(event.expiresAt).toLocaleString("vi-VN") : "Không giới hạn"}</dd></div><div><dt className="text-slate-400">Tải xuống</dt><dd>{event.downloadsEnabled ? "Cho phép" : "Tắt"}</dd></div></dl>
    <h2 className="mt-8 text-xl font-semibold">Xử lý ảnh</h2>
    {status.isLoading && <p className="mt-3 text-slate-400">Đang tải trạng thái…</p>}
    {status.isError && <p className="mt-3" role="alert">Không thể tải tiến độ xử lý ảnh.</p>}
    {status.data && <dl className="mt-3 grid gap-3 text-slate-300 sm:grid-cols-4">
      <div><dt className="text-slate-400">Đang chờ</dt><dd>{status.data.queued}</dd></div>
      <div><dt className="text-slate-400">Đang xử lý</dt><dd>{status.data.processing}</dd></div>
      <div><dt className="text-slate-400">Sẵn sàng</dt><dd>{status.data.ready}</dd></div>
      <div><dt className="text-slate-400">Lỗi</dt><dd>{status.data.failed}</dd></div>
    </dl>}
    {photos.isError && <p className="mt-4" role="alert">Không thể tải danh sách ảnh.</p>}
    {failedPhotos.length > 0 && <section className="mt-6" aria-labelledby="failed-photos-heading">
      <h3 id="failed-photos-heading" className="text-lg font-semibold">Ảnh cần xử lý lại</h3>
      <ul className="mt-3 space-y-2">
        {failedPhotos.map((photo) => <li key={photo.id} className="flex items-center justify-between gap-4 rounded-lg border border-slate-800 p-3">
          <span>{photo.originalFilename}</span>
          {organization.role !== "viewer" && <button type="button" className="rounded-lg border border-cyan-700 px-3 py-2 text-sm" disabled={retry.isPending && retry.variables === photo.id} onClick={() => retry.mutate(photo.id)}>Thử lại</button>}
        </li>)}
      </ul>
      {retry.isError && <p className="mt-3" role="alert">Không thể đưa ảnh vào hàng đợi xử lý lại.</p>}
    </section>}
    {organization.role !== "viewer" && <PhotoUploader organizationId={organization.organizationId} eventId={event.id} />}
  </Shell>;
}
