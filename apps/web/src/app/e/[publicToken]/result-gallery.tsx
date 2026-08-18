"use client";

import { useState } from "react";
import * as api from "@/lib/api";
import type { PublicSearchResult } from "@/lib/api";

const PAGE_SIZE = 12;

type DownloadState = { kind: "idle" } | { kind: "single"; photoId: string } | { kind: "bulk" };

/**
 * Accessible gallery of selfie-search matches with controlled downloads.
 *
 * There is still no public inline image-serving endpoint, so each match renders
 * an accessible placeholder tile keyed by its opaque `photoId`. When the Event
 * allows downloads, each tile exposes a per-photo download action and the
 * section exposes a bounded "download selected" action. Every download is
 * authorized server-side from the public Event token and download policy; the
 * browser only forwards opaque photo identifiers and opens the short-lived,
 * object-scoped links the server returns.
 */
export function ResultGallery({
  publicToken,
  downloadsEnabled,
  results,
  nextCursor,
  page,
  onPageChange,
}: {
  publicToken: string;
  downloadsEnabled: boolean;
  results: PublicSearchResult[];
  nextCursor: string | null;
  page: number;
  onPageChange: (page: number) => void;
}) {
  const totalPages = Math.max(1, Math.ceil(results.length / PAGE_SIZE));
  const current = Math.min(Math.max(page, 1), totalPages);
  const start = (current - 1) * PAGE_SIZE;
  const visible = results.slice(start, start + PAGE_SIZE);

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [download, setDownload] = useState<DownloadState>({ kind: "idle" });
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const busy = download.kind !== "idle";
  const selectedCount = selected.size;
  const overBatchLimit = selectedCount > api.MAX_DOWNLOAD_BATCH;

  function toggleSelected(photoId: string) {
    setSelected((previous) => {
      const next = new Set(previous);
      if (next.has(photoId)) next.delete(photoId);
      else next.add(photoId);
      return next;
    });
  }

  function triggerDownloads(downloads: api.PublicDownload[]) {
    for (const item of downloads) {
      const anchor = document.createElement("a");
      anchor.href = item.url;
      anchor.rel = "noopener";
      anchor.download = "";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
    }
  }

  async function runDownload(state: DownloadState, photoIds: string[]) {
    setDownloadError(null);
    setDownload(state);
    try {
      const response = await api.issuePublicDownloads(publicToken, photoIds);
      triggerDownloads(response.downloads);
    } catch (error) {
      setDownloadError(messageForDownloadError(error));
    } finally {
      setDownload({ kind: "idle" });
    }
  }

  function handleSingle(photoId: string) {
    void runDownload({ kind: "single", photoId }, [photoId]);
  }

  function handleBulk() {
    const photoIds = Array.from(selected);
    if (photoIds.length === 0) {
      setDownloadError("Hãy chọn ít nhất một ảnh để tải.");
      return;
    }
    if (photoIds.length > api.MAX_DOWNLOAD_BATCH) {
      setDownloadError(`Chỉ có thể tải tối đa ${api.MAX_DOWNLOAD_BATCH} ảnh mỗi lần. Vui lòng bớt lựa chọn.`);
      return;
    }
    void runDownload({ kind: "bulk" }, photoIds);
  }

  return (
    <section className="mt-10" aria-labelledby="gallery-heading">
      <h2 id="gallery-heading" className="text-xl font-semibold">
        Tìm thấy {results.length} ảnh
      </h2>
      {downloadsEnabled ? (
        <p className="mt-2 text-sm text-slate-400">
          Chọn ảnh rồi tải xuống, hoặc tải từng ảnh. Liên kết tải có hiệu lực trong thời gian ngắn.
        </p>
      ) : (
        <p className="mt-2 text-sm text-slate-400">Sự kiện này hiện không cho phép tải ảnh.</p>
      )}

      {downloadsEnabled && (
        <div className="mt-4 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={handleBulk}
            disabled={busy || selectedCount === 0 || overBatchLimit}
            className="rounded-lg bg-cyan-400 px-4 py-2 text-sm font-semibold text-slate-950 disabled:opacity-50"
          >
            {download.kind === "bulk" ? "Đang chuẩn bị tải…" : `Tải ảnh đã chọn (${selectedCount})`}
          </button>
          {selectedCount > 0 && (
            <button
              type="button"
              onClick={() => setSelected(new Set())}
              disabled={busy}
              className="rounded-lg border border-slate-700 px-4 py-2 text-sm font-semibold disabled:opacity-50"
            >
              Bỏ chọn
            </button>
          )}
          {overBatchLimit && (
            <span className="text-sm text-rose-400">
              Chỉ có thể tải tối đa {api.MAX_DOWNLOAD_BATCH} ảnh mỗi lần.
            </span>
          )}
        </div>
      )}

      {downloadError && (
        <p role="alert" className="mt-3 text-sm text-rose-400">
          {downloadError}
        </p>
      )}

      <ul className="mt-6 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
        {visible.map((result) => {
          const isSelected = selected.has(result.photoId);
          return (
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
                {downloadsEnabled && (
                  <div className="flex items-center justify-between gap-2 border-t border-slate-800 px-3 py-2">
                    <label className="flex items-center gap-2 text-xs text-slate-300">
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => toggleSelected(result.photoId)}
                        disabled={busy}
                        aria-label={`Chọn ảnh ${result.photoId}`}
                      />
                      Chọn
                    </label>
                    <button
                      type="button"
                      onClick={() => handleSingle(result.photoId)}
                      disabled={busy}
                      className="rounded-md border border-slate-700 px-2 py-1 text-xs font-semibold disabled:opacity-50"
                      aria-label={`Tải ảnh ${result.photoId}`}
                    >
                      {download.kind === "single" && download.photoId === result.photoId ? "Đang tải…" : "Tải ảnh"}
                    </button>
                  </div>
                )}
              </figure>
            </li>
          );
        })}
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

function messageForDownloadError(error: unknown): string {
  if (error instanceof api.PublicDownloadRequestError) {
    switch (error.status) {
      case 400:
        return "Yêu cầu tải không hợp lệ. Vui lòng thử lại.";
      case 404:
        return "Không thể tải ảnh. Sự kiện có thể đã tắt tải hoặc không còn khả dụng.";
      case 429:
        return "Bạn đã tải quá nhiều lần. Vui lòng đợi một lát rồi thử lại.";
      case 503:
        return "Dịch vụ tải đang bận. Vui lòng thử lại sau giây lát.";
    }
  }
  return "Đã xảy ra lỗi khi tải ảnh. Vui lòng thử lại.";
}
