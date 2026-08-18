"use client";

import type { PublicSearchResult } from "@/lib/api";

const PAGE_SIZE = 12;

/**
 * Presentational, accessible gallery of selfie-search matches.
 *
 * There is no public photo-serving endpoint yet (Task 6.4 adds controlled
 * downloads), so each match renders an accessible placeholder tile keyed by its
 * opaque `photoId`. The tile structure is deliberately shaped so a real preview
 * image and download control can be dropped in without changing the layout.
 */
export function ResultGallery({
  results,
  nextCursor,
  page,
  onPageChange,
}: {
  results: PublicSearchResult[];
  nextCursor: string | null;
  page: number;
  onPageChange: (page: number) => void;
}) {
  const totalPages = Math.max(1, Math.ceil(results.length / PAGE_SIZE));
  const current = Math.min(Math.max(page, 1), totalPages);
  const start = (current - 1) * PAGE_SIZE;
  const visible = results.slice(start, start + PAGE_SIZE);

  return (
    <section className="mt-10" aria-labelledby="gallery-heading">
      <h2 id="gallery-heading" className="text-xl font-semibold">
        Tìm thấy {results.length} ảnh
      </h2>
      <p className="mt-2 text-sm text-slate-400">
        Ảnh xem trước và tải xuống sẽ khả dụng khi ảnh sự kiện được mở tải xuống.
      </p>
      <ul className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        {visible.map((result) => (
          <li key={result.photoId}>
            <figure
              className="overflow-hidden rounded-xl border border-slate-800 bg-slate-900"
              aria-label={`Ảnh kết quả ${result.photoId}`}
              title={result.photoId}
            >
              <div
                className="flex aspect-square items-center justify-center bg-slate-800 text-slate-500"
                aria-hidden="true"
              >
                <span className="text-xs tracking-[0.2em] uppercase">Ảnh</span>
              </div>
              <figcaption className="truncate px-3 py-2 text-xs text-slate-400">
                Mã ảnh: {result.photoId.slice(0, 8)}
              </figcaption>
            </figure>
          </li>
        ))}
      </ul>
      {totalPages > 1 && (
        <nav className="mt-6 flex items-center justify-between" aria-label="Phân trang kết quả">
          <button
            type="button"
            onClick={() => onPageChange(current - 1)}
            disabled={current <= 1}
            className="rounded-lg border border-slate-700 px-4 py-2 text-sm font-semibold disabled:opacity-40"
          >
            Trước
          </button>
          <span className="text-sm text-slate-400">
            Trang {current} / {totalPages}
          </span>
          <button
            type="button"
            onClick={() => onPageChange(current + 1)}
            disabled={current >= totalPages}
            className="rounded-lg border border-slate-700 px-4 py-2 text-sm font-semibold disabled:opacity-40"
          >
            Sau
          </button>
        </nav>
      )}
      {nextCursor && (
        <p className="mt-4 text-sm text-slate-400">Còn thêm ảnh khớp ngoài danh sách hiển thị.</p>
      )}
    </section>
  );
}
