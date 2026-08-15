"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import { useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";
import { Shell } from "../../page";

export default function EventSettingsPage() {
  const { eventId } = useParams<{ eventId: string }>();
  const auth = useAuth();
  const organization = auth.currentOrganization;
  const detail = useQuery({ queryKey: ["event", organization?.organizationId, eventId], queryFn: () => api.getEvent(organization!.organizationId, eventId), enabled: Boolean(organization && eventId) });
  if (auth.restoring || detail.isLoading) return <Shell><p>Đang tải cài đặt…</p></Shell>;
  if (!organization || organization.role === "viewer") return <Shell><p>Bạn không có quyền thay đổi sự kiện.</p></Shell>;
  if (!detail.data) return <Shell><p>Sự kiện không khả dụng.</p></Shell>;
  return <SettingsForm key={detail.data.updatedAt} event={detail.data} organizationId={organization.organizationId} />;
}

function SettingsForm({ event, organizationId }: { event: api.Event; organizationId: string }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [name, setName] = useState(event.name);
  const [visibility, setVisibility] = useState<"private" | "public">(event.visibility);
  const update = useMutation({ mutationFn: () => api.updateEvent(organizationId, event.id, { name: name.trim(), visibility }), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["event", organizationId, event.id] }); router.push(`/events/${event.id}`); } });
  const archive = useMutation({ mutationFn: () => api.archiveEvent(organizationId, event.id), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["events", organizationId] }); router.push("/events"); } });
  return <Shell><h1 className="text-3xl font-semibold">Cài đặt sự kiện</h1><form className="mt-8 max-w-xl space-y-5" onSubmit={(submitEvent) => { submitEvent.preventDefault(); if (name.trim()) update.mutate(); }}><label className="block">Tên sự kiện<input value={name} onChange={(changeEvent) => setName(changeEvent.target.value)} className="mt-2 w-full rounded-lg border border-slate-700 bg-slate-900 px-4 py-3" /></label><label className="block">Quyền truy cập<select value={visibility} onChange={(changeEvent) => setVisibility(changeEvent.target.value as "private" | "public")} className="mt-2 w-full rounded-lg border border-slate-700 bg-slate-900 px-4 py-3"><option value="private">Riêng tư</option><option value="public">Công khai</option></select></label>{update.isError && <p role="alert" className="text-rose-300">Không thể cập nhật sự kiện.</p>}<button disabled={!name.trim() || update.isPending} className="rounded-lg bg-cyan-400 px-5 py-3 font-semibold text-slate-950 disabled:opacity-60">{update.isPending ? "Đang lưu…" : "Lưu thay đổi"}</button></form><div className="mt-12 border-t border-slate-800 pt-8"><h2 className="text-lg font-semibold text-rose-300">Lưu trữ sự kiện</h2><p className="mt-2 text-sm text-slate-400">Sự kiện sẽ không còn xuất hiện trong danh sách hoạt động.</p><button disabled={archive.isPending} onClick={() => archive.mutate()} className="mt-4 rounded-lg border border-rose-800 px-4 py-3 text-rose-300">{archive.isPending ? "Đang lưu trữ…" : "Lưu trữ sự kiện"}</button></div></Shell>;
}
