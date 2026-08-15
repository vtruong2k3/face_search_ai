package objectstorage

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/minio/minio-go/v7"
)

type MinIO struct {
	client *minio.Client
	core   minio.Core
	bucket string
}

func NewMinIO(client *minio.Client, bucket string) *MinIO {
	return &MinIO{client: client, core: minio.Core{Client: client}, bucket: bucket}
}

func (m *MinIO) Initiate(ctx context.Context, objectKey, contentType, checksumSHA256 string) (string, error) {
	metadata := map[string]string{}
	if checksumSHA256 != "" {
		metadata["checksum-sha256"] = checksumSHA256
	}
	return m.core.NewMultipartUpload(ctx, m.bucket, objectKey, minio.PutObjectOptions{ContentType: contentType, UserMetadata: metadata})
}

func (m *MinIO) SignPart(ctx context.Context, objectKey, uploadID string, partNumber int, ttl time.Duration) (string, error) {
	parameters := url.Values{}
	parameters.Set("uploadId", uploadID)
	parameters.Set("partNumber", strconv.Itoa(partNumber))
	signed, err := m.client.Presign(ctx, http.MethodPut, m.bucket, objectKey, ttl, parameters)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

func (m *MinIO) Complete(ctx context.Context, objectKey, uploadID string, parts []photo.CompletedPart) error {
	storageParts := make([]minio.CompletePart, len(parts))
	for index, part := range parts {
		storageParts[index] = minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag}
	}
	_, err := m.core.CompleteMultipartUpload(ctx, m.bucket, objectKey, uploadID, storageParts, minio.PutObjectOptions{})
	return err
}

func (m *MinIO) Abort(ctx context.Context, objectKey, uploadID string) error {
	return m.core.AbortMultipartUpload(ctx, m.bucket, objectKey, uploadID)
}

func (m *MinIO) Stat(ctx context.Context, objectKey string) (photo.StoredObject, error) {
	result, err := m.client.StatObject(ctx, m.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return photo.StoredObject{}, err
	}
	checksum := result.UserMetadata["Checksum-Sha256"]
	return photo.StoredObject{ByteSize: result.Size, ContentType: result.ContentType, ChecksumSHA256: checksum}, nil
}

var _ photo.MultipartStorage = (*MinIO)(nil)
