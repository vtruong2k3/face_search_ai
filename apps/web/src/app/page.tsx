const services = [
  { name: "Web", detail: "Next.js application shell", status: "Ready" },
  { name: "API", detail: "Go service boundary", status: "Ready" },
  {
    name: "Face AI",
    detail: "Python inference placeholder",
    status: "Planned",
  },
  { name: "Worker", detail: "Background jobs placeholder", status: "Planned" },
];

export default function Home() {
  return (
    <main className="min-h-screen bg-slate-950 px-6 py-16 text-slate-100">
      <div className="mx-auto max-w-5xl">
        <p className="text-sm font-semibold tracking-[0.25em] text-cyan-400 uppercase">
          Project scaffold
        </p>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight sm:text-6xl">
          Face Search AI
        </h1>
        <p className="mt-6 max-w-2xl text-lg leading-8 text-slate-400">
          Bộ khung frontend và backend đã sẵn sàng. Chưa có nghiệp vụ, model AI
          hoặc dữ liệu production trong giai đoạn này.
        </p>
        <section className="mt-12 grid gap-4 sm:grid-cols-2">
          {services.map((service) => (
            <article
              key={service.name}
              className="rounded-2xl border border-slate-800 bg-slate-900 p-6"
            >
              <div className="flex items-center justify-between gap-4">
                <h2 className="text-xl font-medium">{service.name}</h2>
                <span className="rounded-full bg-cyan-400/10 px-3 py-1 text-xs font-medium text-cyan-300">
                  {service.status}
                </span>
              </div>
              <p className="mt-3 text-slate-400">{service.detail}</p>
            </article>
          ))}
        </section>
      </div>
    </main>
  );
}
