"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { useAuth } from "@/components/providers/auth-provider";
import * as api from "@/lib/api";
import { Shell } from "../page";

const schema = z.object({ name: z.string().trim().min(1, "Nhập tên sự kiện.").max(200), visibility: z.enum(["private", "public"]), downloadsEnabled: z.boolean() });
type Values = z.infer<typeof schema>;

export default function NewEventPage() {
  const auth = useAuth();
  const router = useRouter();
  const queryClient = useQueryClient();
  const organization = auth.currentOrganization;
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: "", visibility: "private", downloadsEnabled: false } });
  const create = useMutation({ mutationFn: (values: Values) => api.createEvent(organization!.organizationId, { ...values, expiresAt: null, matchThreshold: null }), onSuccess: async (event) => { await queryClient.invalidateQueries({ queryKey: ["events", organization!.organizationId] }); router.push(`/events/${event.id}`); } });

  if (auth.restoring || auth.organizationsLoading) return <Shell><p>Đang tải…</p></Shell>;
  if (!organization || organization.role === "viewer") return <Shell><p>Bạn không có quyền tạo sự kiện.</p></Shell>;

  return <Shell><h1 className="text-3xl font-semibold">Tạo sự kiện</h1><form className="mt-8 max-w-xl space-y-5" noValidate onSubmit={form.handleSubmit((values) => create.mutate(values))}>
    <label className="block">Tên sự kiện<input className="mt-2 w-full rounded-lg border border-slate-700 bg-slate-900 px-4 py-3" {...form.register("name")} /></label>
    {form.formState.errors.name && <p className="text-sm text-rose-300">{form.formState.errors.name.message}</p>}
    <label className="block">Quyền truy cập<select className="mt-2 w-full rounded-lg border border-slate-700 bg-slate-900 px-4 py-3" {...form.register("visibility")}><option value="private">Riêng tư</option><option value="public">Công khai</option></select></label>
    <label className="flex items-center gap-3"><input type="checkbox" {...form.register("downloadsEnabled")} /> Cho phép tải xuống</label>
    {create.isError && <p role="alert" className="text-sm text-rose-300">Không thể tạo sự kiện. Kiểm tra thông tin và thử lại.</p>}
    <button disabled={create.isPending} className="rounded-lg bg-cyan-400 px-5 py-3 font-semibold text-slate-950 disabled:opacity-60">{create.isPending ? "Đang tạo…" : "Tạo sự kiện"}</button>
  </form></Shell>;
}
