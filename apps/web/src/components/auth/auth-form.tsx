"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { useAuth } from "@/components/providers/auth-provider";

const schema = z.object({
  email: z.string().email("Nhập địa chỉ email hợp lệ."),
  password: z.string().min(12, "Mật khẩu phải có ít nhất 12 ký tự.").max(128, "Mật khẩu tối đa 128 ký tự."),
});
type FormValues = z.infer<typeof schema>;

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const auth = useAuth();
  const router = useRouter();
  const { register, handleSubmit, setError, formState: { errors, isSubmitting } } = useForm<FormValues>({ resolver: zodResolver(schema) });
  const isLogin = mode === "login";

  async function submit(values: FormValues) {
    try {
      await (isLogin ? auth.login(values) : auth.register(values));
      router.push("/account");
    } catch {
      setError("root", { message: "Yêu cầu xác thực bị từ chối." });
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-950 px-6 py-16 text-slate-100">
      <section className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8">
        <p className="text-sm font-semibold tracking-[0.2em] text-cyan-400 uppercase">Face Search AI</p>
        <h1 className="mt-3 text-3xl font-semibold">{isLogin ? "Đăng nhập" : "Tạo tài khoản"}</h1>
        <form className="mt-8 space-y-5" onSubmit={handleSubmit(submit)} noValidate>
          <div>
            <label className="mb-2 block text-sm" htmlFor="email">Email</label>
            <input id="email" type="email" autoComplete="email" className="w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3" {...register("email")} />
            {errors.email && <p className="mt-2 text-sm text-rose-400">{errors.email.message}</p>}
          </div>
          <div>
            <label className="mb-2 block text-sm" htmlFor="password">Mật khẩu</label>
            <input id="password" type="password" autoComplete={isLogin ? "current-password" : "new-password"} className="w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3" {...register("password")} />
            {errors.password && <p className="mt-2 text-sm text-rose-400">{errors.password.message}</p>}
          </div>
          {errors.root && <p role="alert" className="text-sm text-rose-400">{errors.root.message}</p>}
          <button disabled={isSubmitting} className="w-full rounded-lg bg-cyan-400 px-4 py-3 font-semibold text-slate-950 disabled:opacity-60">
            {isSubmitting ? "Đang xử lý…" : isLogin ? "Đăng nhập" : "Đăng ký"}
          </button>
        </form>
        <p className="mt-6 text-sm text-slate-400">
          {isLogin ? "Chưa có tài khoản?" : "Đã có tài khoản?"}{" "}
          <Link className="text-cyan-300" href={isLogin ? "/register" : "/login"}>{isLogin ? "Đăng ký" : "Đăng nhập"}</Link>
        </p>
      </section>
    </main>
  );
}
