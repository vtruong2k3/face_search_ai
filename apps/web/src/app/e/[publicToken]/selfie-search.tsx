"use client";

import { useEffect, useRef, useState } from "react";
import * as api from "@/lib/api";
import { ResultGallery } from "./result-gallery";

// Version identifier for the consent copy shown below. The API requires a
// non-empty consent version (1–64 chars); bump this when the consent text
// materially changes so server-side records stay meaningful.
const CONSENT_VERSION = "2026-08-15";

const ACCEPTED_TYPES = ["image/jpeg", "image/png", "image/webp"];
const MAX_BYTES = 10 * 1024 * 1024;

type Status = "idle" | "searching";

/**
 * Ephemeral customer selfie search for a public Event.
 *
 * Privacy: the selfie is never written to localStorage, sessionStorage, or
 * IndexedDB. The only in-memory references are the selected `File` and a preview
 * object URL; both are released (and the file input reset) as soon as a search
 * attempt settles, so the image does not outlive the active flow.
 */
export function SelfieSearch({ publicToken, downloadsEnabled }: { publicToken: string; downloadsEnabled: boolean }) {
  const [consent, setConsent] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [status, setStatus] = useState<Status>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [results, setResults] = useState<api.PublicSearchResult[] | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [page, setPage] = useState(1);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const previewUrlRef = useRef<string | null>(null);

  // Release any outstanding preview object URL when the component unmounts.
  useEffect(() => {
    return () => {
      if (previewUrlRef.current) {
        URL.revokeObjectURL(previewUrlRef.current);
        previewUrlRef.current = null;
      }
    };
  }, []);

  function releasePreview() {
    if (previewUrlRef.current) {
      URL.revokeObjectURL(previewUrlRef.current);
      previewUrlRef.current = null;
    }
    setPreviewUrl(null);
  }

  function resetFileInput() {
    if (fileInputRef.current) fileInputRef.current.value = "";
  }

  function clearSelfie() {
    releasePreview();
    resetFileInput();
    setFile(null);
  }

  function handleFileChange(event: React.ChangeEvent<HTMLInputElement>) {
    const selected = event.target.files?.[0] ?? null;
    releasePreview();
    setErrorMessage(null);
    setFile(null);
    if (!selected) return;
    if (!ACCEPTED_TYPES.includes(selected.type)) {
      setErrorMessage("Định dạng ảnh không được hỗ trợ. Vui lòng dùng JPEG, PNG hoặc WebP.");
      resetFileInput();
      return;
    }
    if (selected.size > MAX_BYTES) {
      setErrorMessage("Ảnh vượt quá 10 MB. Vui lòng chọn ảnh nhỏ hơn.");
      resetFileInput();
      return;
    }
    const url = URL.createObjectURL(selected);
    previewUrlRef.current = url;
    setPreviewUrl(url);
    setFile(selected);
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!consent) {
      setErrorMessage("Bạn cần đồng ý trước khi tìm kiếm.");
      return;
    }
    if (!file) {
      setErrorMessage("Vui lòng chọn ảnh selfie.");
      return;
    }
    setStatus("searching");
    setErrorMessage(null);
    try {
      const response = await api.searchPublicEvent(publicToken, file, CONSENT_VERSION);
      setResults(response.results);
      setNextCursor(response.nextCursor);
      setPage(1);
    } catch (error) {
      setErrorMessage(messageForError(error));
    } finally {
      setStatus("idle");
      // Do not retain the selfie beyond the search that used it.
      clearSelfie();
    }
  }

  const searching = status === "searching";
  const canSearch = consent && file !== null && !searching;

  return (
    <section className="mt-10 text-left" aria-labelledby="search-heading">
      <h2 id="search-heading" className="text-xl font-semibold">Tìm ảnh của bạn</h2>
      <p className="mt-2 text-sm text-slate-400">
        Chụp hoặc tải lên một ảnh selfie rõ nét chỉ có một khuôn mặt. Ảnh chỉ được xử lý cho lần tìm này và không được lưu lại.
      </p>
      <form className="mt-6 space-y-5" onSubmit={handleSubmit} noValidate>
        <div>
          <label className="mb-2 block text-sm" htmlFor="selfie">Ảnh selfie</label>
          <input
            id="selfie"
            ref={fileInputRef}
            type="file"
            accept="image/jpeg,image/png,image/webp"
            capture="user"
            onChange={handleFileChange}
            className="w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3 text-sm file:mr-4 file:rounded-md file:border-0 file:bg-slate-800 file:px-3 file:py-1 file:text-slate-100"
          />
        </div>
        {previewUrl && (
          <div>
            <p className="mb-2 text-sm text-slate-400">Xem trước</p>
            {/* Local object-URL preview only; released as soon as the search settles. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={previewUrl} alt="Xem trước ảnh selfie" className="h-40 w-40 rounded-lg border border-slate-800 object-cover" />
          </div>
        )}
        <label className="flex items-start gap-3 text-sm text-slate-300">
          <input
            type="checkbox"
            checked={consent}
            onChange={(event) => setConsent(event.target.checked)}
            className="mt-1"
          />
          <span>
            Tôi đồng ý cho hệ thống phân tích khuôn mặt trong ảnh selfie để tìm ảnh khớp trong sự kiện này. Ảnh selfie không được lưu trữ sau khi tìm kiếm.
          </span>
        </label>
        {errorMessage && (
          <p role="alert" className="text-sm text-rose-400">{errorMessage}</p>
        )}
        {searching && (
          <p role="status" className="text-sm text-cyan-300">Đang tìm kiếm…</p>
        )}
        <button
          type="submit"
          disabled={!canSearch}
          className="w-full rounded-lg bg-cyan-400 px-4 py-3 font-semibold text-slate-950 disabled:opacity-60"
        >
          {searching ? "Đang tìm kiếm…" : "Tìm ảnh"}
        </button>
      </form>
      {results !== null && !searching && (
        results.length === 0 ? (
          <p className="mt-8 rounded-xl border border-slate-800 bg-slate-900 p-6 text-center text-sm text-slate-400">
            Không tìm thấy ảnh phù hợp với khuôn mặt của bạn.
          </p>
        ) : (
          <ResultGallery publicToken={publicToken} downloadsEnabled={downloadsEnabled} results={results} nextCursor={nextCursor} page={page} onPageChange={setPage} />
        )
      )}
    </section>
  );
}

function messageForError(error: unknown): string {
  if (error instanceof api.PublicSearchRequestError) {
    if (error.code) {
      switch (error.code) {
        case "consent_required":
          return "Bạn cần đồng ý trước khi tìm kiếm.";
        case "invalid_image":
          return "Không đọc được ảnh. Vui lòng chọn ảnh khác.";
        case "unsupported_media_type":
          return "Định dạng ảnh không được hỗ trợ. Vui lòng dùng JPEG, PNG hoặc WebP.";
        case "selfie_too_large":
          return "Ảnh vượt quá 10 MB. Vui lòng chọn ảnh nhỏ hơn.";
        case "face_count_zero":
          return "Không phát hiện khuôn mặt nào. Hãy dùng ảnh rõ nét có đúng một khuôn mặt.";
        case "face_count_multiple":
          return "Ảnh có nhiều khuôn mặt. Hãy dùng ảnh chỉ có một khuôn mặt.";
        case "invalid_cursor":
          return "Trang kết quả không còn hợp lệ. Vui lòng tìm lại.";
      }
    }
    switch (error.status) {
      case 400:
        return "Yêu cầu không hợp lệ. Vui lòng thử lại.";
      case 404:
        return "Sự kiện không khả dụng.";
      case 413:
        return "Ảnh vượt quá giới hạn cho phép. Vui lòng chọn ảnh nhỏ hơn.";
      case 429:
        return "Bạn đã thử quá nhiều lần. Vui lòng đợi một lát rồi thử lại.";
      case 503:
        return "Dịch vụ đang bận. Vui lòng thử lại sau giây lát.";
    }
  }
  return "Đã xảy ra lỗi khi tìm kiếm. Vui lòng thử lại.";
}
