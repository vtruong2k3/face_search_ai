package config

import (
	"testing"
	"time"
)

func TestLoadDatabaseSettings(t *testing.T) {
	t.Setenv("DATABASE_MAX_CONNECTIONS", "24")
	t.Setenv("DATABASE_SCHEMA_VERSION", "7")
	cfg := Load()
	if cfg.DatabaseMaxConns != 24 {
		t.Fatalf("DatabaseMaxConns = %d", cfg.DatabaseMaxConns)
	}
	if cfg.SchemaVersion != 7 {
		t.Fatalf("SchemaVersion = %d", cfg.SchemaVersion)
	}
}

func TestLoadDatabaseSettingsUsesSafeDefaults(t *testing.T) {
	t.Setenv("DATABASE_MAX_CONNECTIONS", "0")
	t.Setenv("DATABASE_SCHEMA_VERSION", "invalid")
	cfg := Load()
	if cfg.DatabaseMaxConns != 10 {
		t.Fatalf("DatabaseMaxConns = %d", cfg.DatabaseMaxConns)
	}
	if cfg.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d", cfg.SchemaVersion)
	}
}

func TestLoadPhotoUploadSettings(t *testing.T) {
	t.Setenv("PHOTO_MAX_BYTE_SIZE", "52428800")
	t.Setenv("PHOTO_UPLOAD_PART_SIZE", "8388608")
	t.Setenv("PHOTO_UPLOAD_MAX_PARTS", "100")
	t.Setenv("PHOTO_UPLOAD_SIGN_TTL", "7m")
	t.Setenv("PHOTO_UPLOAD_SESSION_TTL", "12h")
	cfg := Load()
	if cfg.PhotoMaxByteSize != 52428800 || cfg.PhotoUploadPartSize != 8388608 || cfg.PhotoUploadMaxParts != 100 {
		t.Fatalf("upload bounds = %d/%d/%d", cfg.PhotoMaxByteSize, cfg.PhotoUploadPartSize, cfg.PhotoUploadMaxParts)
	}
	if cfg.PhotoUploadSignTTL != 7*time.Minute || cfg.PhotoUploadSessionTTL != 12*time.Hour {
		t.Fatalf("upload TTLs = %s/%s", cfg.PhotoUploadSignTTL, cfg.PhotoUploadSessionTTL)
	}
}

func TestLoadPhotoUploadSettingsUsesSafeDefaults(t *testing.T) {
	t.Setenv("PHOTO_MAX_BYTE_SIZE", "0")
	t.Setenv("PHOTO_UPLOAD_PART_SIZE", "1")
	t.Setenv("PHOTO_UPLOAD_MAX_PARTS", "10001")
	t.Setenv("PHOTO_UPLOAD_SIGN_TTL", "25h")
	t.Setenv("PHOTO_UPLOAD_SESSION_TTL", "30s")
	cfg := Load()
	if cfg.PhotoMaxByteSize != 100*1024*1024 || cfg.PhotoUploadPartSize != 8*1024*1024 || cfg.PhotoUploadMaxParts != 1000 {
		t.Fatalf("safe upload bounds = %d/%d/%d", cfg.PhotoMaxByteSize, cfg.PhotoUploadPartSize, cfg.PhotoUploadMaxParts)
	}
	if cfg.PhotoUploadSignTTL != 10*time.Minute || cfg.PhotoUploadSessionTTL != 24*time.Hour {
		t.Fatalf("safe upload TTLs = %s/%s", cfg.PhotoUploadSignTTL, cfg.PhotoUploadSessionTTL)
	}
}
