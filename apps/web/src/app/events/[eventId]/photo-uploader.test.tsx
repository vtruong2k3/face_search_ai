import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const installPlugin = vi.fn();
const on = vi.fn();
const destroy = vi.fn();
const getFiles = vi.fn(() => []);
let awsOptions: Record<string, (...args: never[]) => unknown>;

vi.mock("@uppy/core", () => ({
  default: class {
    use(plugin: unknown, options: Record<string, (...args: never[]) => unknown>) {
      installPlugin(plugin, options);
      if (typeof options.createMultipartUpload === "function") awsOptions = options;
      return this;
    }
    on = on;
    destroy = destroy;
    getFiles = getFiles;
  },
}));
vi.mock("@uppy/aws-s3", () => ({ default: class AwsS3 {} }));
vi.mock("@uppy/dashboard", () => ({ default: class Dashboard {} }));
vi.mock("@/lib/api", () => ({
  createPhoto: vi.fn(),
  initiatePhotoUpload: vi.fn(),
  signPhotoUploadPart: vi.fn(),
  completePhotoUpload: vi.fn(),
  abortPhotoUpload: vi.fn(),
}));

import * as api from "@/lib/api";
import { createPhotoUploader, PhotoUploader } from "./photo-uploader";

const file = { id: "file-1", name: "photo.jpg", type: "image/jpeg", size: 9, progress: {} };

describe("photo uploader", () => {
  beforeEach(() => {
    installPlugin.mockClear();
    on.mockClear();
    destroy.mockClear();
    vi.mocked(api.createPhoto).mockReset();
    vi.mocked(api.initiatePhotoUpload).mockReset();
    vi.mocked(api.signPhotoUploadPart).mockReset();
    vi.mocked(api.completePhotoUpload).mockReset();
    vi.mocked(api.abortPhotoUpload).mockReset();
    createPhotoUploader("org-1", "event-1", document.createElement("div"));
  });

  it("maps the tenant-scoped multipart lifecycle", async () => {
    vi.mocked(api.createPhoto).mockResolvedValue({ id: "photo-1" } as api.Photo);
    vi.mocked(api.initiatePhotoUpload).mockResolvedValue({ photoId: "photo-1", uploadId: "upload-1", partSize: 8, partCount: 2, expiresAt: "2026-08-16T00:00:00Z" });
    vi.mocked(api.signPhotoUploadPart).mockResolvedValue({ partNumber: 1, url: "https://storage.test/part", expiresAt: "2026-08-15T12:10:00Z" });

    expect(await awsOptions.createMultipartUpload(file as never)).toEqual({ uploadId: "upload-1", key: "photo-1" });
    expect(api.createPhoto).toHaveBeenCalledWith("org-1", "event-1", { originalFilename: "photo.jpg", contentType: "image/jpeg", byteSize: 9 });

    expect(await awsOptions.signPart(file as never, { uploadId: "upload-1", key: "photo-1", partNumber: 1 } as never)).toEqual({ method: "PUT", url: "https://storage.test/part" });
    expect(api.signPhotoUploadPart).toHaveBeenCalledWith("org-1", "event-1", "photo-1", "upload-1", 1, undefined);

    await awsOptions.completeMultipartUpload(file as never, { uploadId: "upload-1", key: "photo-1", parts: [{ PartNumber: 1, ETag: "etag-1" }], signal: new AbortController().signal } as never);
    expect(api.completePhotoUpload).toHaveBeenCalledWith("org-1", "event-1", "photo-1", "upload-1", [{ partNumber: 1, etag: "etag-1" }], expect.any(AbortSignal));
  });

  it("destroys tenant-scoped state when the scope changes", () => {
    const view = render(<PhotoUploader organizationId="org-1" eventId="event-1" />);
    view.rerender(<PhotoUploader organizationId="org-2" eventId="event-2" />);
    expect(destroy).toHaveBeenCalledTimes(1);
    view.unmount();
    expect(destroy).toHaveBeenCalledTimes(2);
  });

  it("aborts the exact server-authorized upload", async () => {
    vi.mocked(api.createPhoto).mockResolvedValue({ id: "photo-1" } as api.Photo);
    vi.mocked(api.initiatePhotoUpload).mockResolvedValue({ photoId: "photo-1", uploadId: "upload-1", partSize: 8, partCount: 2, expiresAt: "2026-08-16T00:00:00Z" });
    await awsOptions.createMultipartUpload(file as never);
    await awsOptions.abortMultipartUpload(file as never, { uploadId: "upload-1", key: "photo-1" } as never);
    expect(api.abortPhotoUpload).toHaveBeenCalledWith("org-1", "event-1", "photo-1", "upload-1", undefined);
  });
});
