import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "@/lib/api";
import { SelfieSearch } from "./selfie-search";

const searchPublicEvent = vi.fn();

vi.mock("@/lib/api", async () => ({
  searchPublicEvent: (...args: unknown[]) => searchPublicEvent(...args),
  PublicSearchRequestError: class PublicSearchRequestError extends Error {
    status: number;
    code: string | null;
    constructor(status: number, code: string | null) {
      super("Public search request rejected.");
      this.name = "PublicSearchRequestError";
      this.status = status;
      this.code = code;
    }
  },
}));

const createObjectURL = vi.fn(() => "blob:preview");
const revokeObjectURL = vi.fn();

function makeFile(type = "image/jpeg", name = "selfie.jpg") {
  return new File(["selfie-bytes"], name, { type });
}

function selectFile(file: File) {
  fireEvent.change(screen.getByLabelText("Ảnh selfie"), { target: { files: [file] } });
}

function checkConsent() {
  fireEvent.click(screen.getByRole("checkbox"));
}

describe("SelfieSearch", () => {
  beforeEach(() => {
    searchPublicEvent.mockReset();
    createObjectURL.mockClear();
    revokeObjectURL.mockClear();
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = revokeObjectURL;
  });

  it("requires consent before a search can run", async () => {
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    const button = screen.getByRole("button", { name: "Tìm ảnh" });
    expect(button).toBeDisabled();
    checkConsent();
    expect(button).toBeEnabled();
    expect(searchPublicEvent).not.toHaveBeenCalled();
  });

  it("shows a preview for a valid selfie and rejects unsupported types", () => {
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    expect(screen.getByAltText("Xem trước ảnh selfie")).toBeInTheDocument();
    selectFile(makeFile("application/pdf", "notes.pdf"));
    expect(screen.getByRole("alert")).toHaveTextContent("Định dạng ảnh không được hỗ trợ. Vui lòng dùng JPEG, PNG hoặc WebP.");
    expect(screen.queryByAltText("Xem trước ảnh selfie")).not.toBeInTheDocument();
  });

  it("renders the match count and result tiles on success", async () => {
    searchPublicEvent.mockResolvedValue({ results: [{ photoId: "aaaaaaaa-1111" }, { photoId: "bbbbbbbb-2222" }], nextCursor: null });
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByText("Tìm thấy 2 ảnh")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    await waitFor(() => expect(searchPublicEvent).toHaveBeenCalledWith("token-1", expect.any(File), "2026-08-15"));
  });

  it("shows a no-results state when nothing matches", async () => {
    searchPublicEvent.mockResolvedValue({ results: [], nextCursor: null });
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByText("Không tìm thấy ảnh phù hợp với khuôn mặt của bạn.")).toBeInTheDocument();
  });

  it("surfaces the typed single-face errors", async () => {
    render(<SelfieSearch publicToken="token-1" />);

    searchPublicEvent.mockRejectedValueOnce(new api.PublicSearchRequestError(422, "face_count_zero"));
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Không phát hiện khuôn mặt nào. Hãy dùng ảnh rõ nét có đúng một khuôn mặt.");

    searchPublicEvent.mockRejectedValueOnce(new api.PublicSearchRequestError(422, "face_count_multiple"));
    selectFile(makeFile());
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Ảnh có nhiều khuôn mặt. Hãy dùng ảnh chỉ có một khuôn mặt.");
  });

  it("maps status-only failures to a safe message without leaking internals", async () => {
    searchPublicEvent.mockRejectedValue(new api.PublicSearchRequestError(503, null));
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Dịch vụ đang bận. Vui lòng thử lại sau giây lát.");
  });

  it("does not persist the selfie: revokes the object URL, clears the input, and never touches web storage", async () => {
    const setLocal = vi.spyOn(Storage.prototype, "setItem");
    searchPublicEvent.mockResolvedValue({ results: [{ photoId: "aaaaaaaa-1111" }], nextCursor: null });
    render(<SelfieSearch publicToken="token-1" />);
    const input = screen.getByLabelText("Ảnh selfie") as HTMLInputElement;
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    await screen.findByText("Tìm thấy 1 ảnh");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:preview");
    expect(input.value).toBe("");
    expect(screen.queryByAltText("Xem trước ảnh selfie")).not.toBeInTheDocument();
    expect(setLocal).not.toHaveBeenCalled();
    setLocal.mockRestore();
  });

  it("paginates large result sets client-side", async () => {
    const results = Array.from({ length: 15 }, (_, index) => ({ photoId: `photo-${index}` }));
    searchPublicEvent.mockResolvedValue({ results, nextCursor: null });
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByText("Trang 1 / 2")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    fireEvent.click(screen.getByRole("button", { name: "Sau" }));
    expect(screen.getByText("Trang 2 / 2")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
  });

  it("shows a searching state while the request is in flight", async () => {
    let resolveSearch: (value: api.PublicSearchResponse) => void = () => {};
    searchPublicEvent.mockImplementation(() => new Promise((resolve) => { resolveSearch = resolve; }));
    render(<SelfieSearch publicToken="token-1" />);
    selectFile(makeFile());
    checkConsent();
    fireEvent.click(screen.getByRole("button", { name: "Tìm ảnh" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Đang tìm kiếm…");
    resolveSearch({ results: [], nextCursor: null });
    expect(await screen.findByText("Không tìm thấy ảnh phù hợp với khuôn mặt của bạn.")).toBeInTheDocument();
  });
});
