package objectstorage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOIntegrationMultipartLifecycle(t *testing.T) {
	endpoint := os.Getenv("API_MINIO_INTEGRATION_ENDPOINT")
	if endpoint == "" {
		t.Skip("API_MINIO_INTEGRATION_ENDPOINT is not set")
	}
	accessKey := os.Getenv("API_MINIO_INTEGRATION_ACCESS_KEY")
	secretKey := os.Getenv("API_MINIO_INTEGRATION_SECRET_KEY")
	bucket := os.Getenv("API_MINIO_INTEGRATION_BUCKET")
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, "")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		exists, existsErr := client.BucketExists(ctx, bucket)
		if existsErr != nil || !exists {
			t.Fatalf("make bucket: %v", err)
		}
	}
	adapter := NewMinIO(client, bucket)
	objectKey := "integration/multipart-object"
	t.Cleanup(func() { _ = client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{}) })

	uploadID, err := adapter.Initiate(ctx, objectKey, "image/jpeg", "")
	if err != nil {
		t.Fatal(err)
	}
	partURL, err := adapter.SignPart(ctx, objectKey, uploadID, 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{1}, 5*1024*1024)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, partURL, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", response.StatusCode)
	}
	etag := response.Header.Get("ETag")
	if err := adapter.Complete(ctx, objectKey, uploadID, []photo.CompletedPart{{PartNumber: 1, ETag: etag}}); err != nil {
		t.Fatal(err)
	}
	stored, err := adapter.Stat(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ByteSize != int64(len(payload)) || stored.ContentType != "image/jpeg" {
		t.Fatalf("stored = %#v", stored)
	}

	abortedUploadID, err := adapter.Initiate(ctx, objectKey+"-aborted", "image/jpeg", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Abort(ctx, objectKey+"-aborted", abortedUploadID); err != nil {
		t.Fatal(err)
	}
}
