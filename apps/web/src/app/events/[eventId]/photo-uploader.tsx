"use client";

import Uppy, { type Body, type Meta, type UppyFile } from "@uppy/core";
import AwsS3, { type AwsS3Part } from "@uppy/aws-s3";
import Dashboard from "@uppy/dashboard";
import { useEffect, useRef } from "react";
import * as api from "@/lib/api";

type UploadMeta = Meta;
type UploadBody = Body;
type UploadFile = UppyFile<UploadMeta, UploadBody>;
type UploadState = { photoId: string; partSize: number };

export function createPhotoUploader(organizationId: string, eventId: string, target: HTMLElement) {
  const uploads = new Map<string, UploadState>();
  const partSizes = new Map<number, number>();
  const uppy = new Uppy<UploadMeta, UploadBody>({
    autoProceed: false,
    restrictions: {
      allowedFileTypes: ["image/jpeg", "image/png", "image/webp"],
      maxFileSize: 100 * 1024 * 1024,
    },
  });

  uppy.use(Dashboard, {
    target,
    inline: true,
    theme: "dark",
    note: "JPEG, PNG hoặc WebP · tối đa 100 MiB mỗi ảnh",
    proudlyDisplayPoweredByUppy: false,
    showRemoveButtonAfterComplete: true,
  });
  uppy.use(AwsS3, {
    shouldUseMultipart: true,
    limit: 3,
    retryDelays: [0, 1000, 3000, 5000],
    getChunkSize(file) {
      return partSizes.get(file.size) ?? 8 * 1024 * 1024;
    },
    async createMultipartUpload(file) {
      if (!file.type || !file.size || !isAcceptedContentType(file.type)) throw new Error("Ảnh không hợp lệ.");
      const photo = await api.createPhoto(organizationId, eventId, {
        originalFilename: file.name,
        contentType: file.type,
        byteSize: file.size,
      });
      const upload = await api.initiatePhotoUpload(organizationId, eventId, photo.id);
      uploads.set(file.id, { photoId: photo.id, partSize: upload.partSize });
      partSizes.set(file.size, upload.partSize);
      return { uploadId: upload.uploadId, key: photo.id };
    },
    async signPart(file, options) {
      const state = requireUpload(uploads, file);
      const signed = await api.signPhotoUploadPart(organizationId, eventId, state.photoId, options.uploadId, options.partNumber, options.signal);
      return { method: "PUT", url: signed.url };
    },
    listParts() {
      return [];
    },
    async completeMultipartUpload(file, options) {
      const state = requireUpload(uploads, file);
      const parts = normalizeParts(options.parts);
      await api.completePhotoUpload(organizationId, eventId, state.photoId, options.uploadId, parts, options.signal);
      return {};
    },
    async abortMultipartUpload(file, options) {
      const state = uploads.get(file.id);
      if (!state) return;
      if (!options.uploadId) throw new Error("Phiên hủy tải ảnh không còn khả dụng.");
      await api.abortPhotoUpload(organizationId, eventId, state.photoId, options.uploadId, options.signal);
      uploads.delete(file.id);
    },
  });
  uppy.on("complete", (result) => {
    for (const file of result.successful ?? []) uploads.delete(file.id);
  });
  return uppy;
}

function isAcceptedContentType(value: string): value is api.CreatePhoto["contentType"] {
  return value === "image/jpeg" || value === "image/png" || value === "image/webp";
}

function requireUpload(uploads: Map<string, UploadState>, file: UploadFile) {
  const state = uploads.get(file.id);
  if (!state) throw new Error("Phiên tải ảnh không còn khả dụng.");
  return state;
}

function normalizeParts(parts: AwsS3Part[]): api.CompletedUploadPart[] {
  return parts.map((part) => {
    if (!part.PartNumber || !part.ETag) throw new Error("Danh sách phần tải lên không hợp lệ.");
    return { partNumber: part.PartNumber, etag: part.ETag };
  });
}

export function PhotoUploader({ organizationId, eventId }: { organizationId: string; eventId: string }) {
  const target = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!target.current) return;
    const uppy = createPhotoUploader(organizationId, eventId, target.current);
    return () => uppy.destroy();
  }, [organizationId, eventId]);

  return <section className="mt-8" aria-labelledby="photo-upload-heading">
    <h2 id="photo-upload-heading" className="text-xl font-semibold">Tải ảnh lên</h2>
    <p className="mt-2 text-sm text-slate-400">Ảnh được gửi trực tiếp tới kho lưu trữ riêng tư. Bạn có thể tạm dừng, tiếp tục, thử lại hoặc hủy.</p>
    <div className="mt-4" ref={target} />
  </section>;
}
