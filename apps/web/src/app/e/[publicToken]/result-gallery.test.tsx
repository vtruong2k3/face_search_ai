import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResultGallery } from "./result-gallery";

const issuePublicDownloads = vi.fn();

vi.mock("@/lib/api", async () => ({
  issuePublicDownloads: (...args: unknown[]) => issuePublicDownloads(...args),
  MAX_DOWNLOAD_BATCH: 2,
  PublicDownloadRequestError: class PublicDownloadRequestError extends Error {
    status: number;
    constructor(status: number) {
      super("Public download request rejected.");
      this.name = "PublicDownloadRequestError";
      this.status = status;
    }
  },
}));

function results(count: number) {
  return Array.from({ length: count }, (_, index) => ({ photoId: `photo-${index}` }));
}

function renderGallery(props: Partial<React.ComponentProps<typeof ResultGallery>> = {}) {
  return render(
    <ResultGallery
      publicToken="token-1"
      downloadsEnabled
      results={results(3)}
      nextCursor={null}
      page={1}
      onPageChange={() => {}}
      {...props}
    />,
  );
}

describe("ResultGallery downloads", () => {
  beforeEach(() => {
    issuePublicDownloads.mockReset();
    vi.restoreAllMocks();
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  });

  it("hides download controls and explains when downloads are disabled", () => {
    renderGallery({ downloadsEnabled: false });
    expect(screen.getByText("Sự kiện này hiện không cho phép tải ảnh.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Tải ảnh photo-0/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("downloads a single photo through the authorized endpoint", async () => {
    issuePublicDownloads.mockResolvedValue({ downloads: [{ photoId: "photo-0", url: "https://minio.example/a", expiresAt: "2026-08-15T00:00:00Z" }] });
    renderGallery();
    fireEvent.click(screen.getByRole("button", { name: "Tải ảnh photo-0" }));
    await waitFor(() => expect(issuePublicDownloads).toHaveBeenCalledWith("token-1", ["photo-0"]));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1);
  });

  it("downloads a bounded selection in one request", async () => {
    issuePublicDownloads.mockResolvedValue({
      downloads: [
        { photoId: "photo-0", url: "https://minio.example/a", expiresAt: "2026-08-15T00:00:00Z" },
        { photoId: "photo-1", url: "https://minio.example/b", expiresAt: "2026-08-15T00:00:00Z" },
      ],
    });
    renderGallery();
    fireEvent.click(screen.getByRole("checkbox", { name: "Chọn ảnh photo-0" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Chọn ảnh photo-1" }));
    fireEvent.click(screen.getByRole("button", { name: "Tải ảnh đã chọn (2)" }));
    await waitFor(() => expect(issuePublicDownloads).toHaveBeenCalledWith("token-1", ["photo-0", "photo-1"]));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(2);
  });

  it("blocks a selection larger than the batch limit without calling the API", () => {
    renderGallery();
    fireEvent.click(screen.getByRole("checkbox", { name: "Chọn ảnh photo-0" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Chọn ảnh photo-1" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Chọn ảnh photo-2" }));
    // The bulk button is disabled once the selection exceeds the limit; a guard
    // message appears and no request is made.
    expect(screen.getByText("Chỉ có thể tải tối đa 2 ảnh mỗi lần.")).toBeInTheDocument();
    expect(issuePublicDownloads).not.toHaveBeenCalled();
  });

  it("maps a rate-limited download to a safe localized message", async () => {
    const { PublicDownloadRequestError } = await import("@/lib/api");
    issuePublicDownloads.mockRejectedValue(new PublicDownloadRequestError(429));
    renderGallery();
    fireEvent.click(screen.getByRole("button", { name: "Tải ảnh photo-0" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Bạn đã tải quá nhiều lần. Vui lòng đợi một lát rồi thử lại.");
  });

  it("maps a disabled/unavailable download to a safe localized message", async () => {
    const { PublicDownloadRequestError } = await import("@/lib/api");
    issuePublicDownloads.mockRejectedValue(new PublicDownloadRequestError(404));
    renderGallery();
    fireEvent.click(screen.getByRole("button", { name: "Tải ảnh photo-0" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Không thể tải ảnh. Sự kiện có thể đã tắt tải hoặc không còn khả dụng.");
  });
});
