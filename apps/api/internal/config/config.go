package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host                  string
	Port                  string
	DatabaseURL           string
	DatabaseMaxConns      int32
	SchemaVersion         int64
	RedisURL              string
	QdrantURL             string
	MinIOEndpoint         string
	MinIOAccessKey        string
	MinIOSecretKey        string
	MinIOUseTLS           bool
	MinIOBucket           string
	PhotoMaxByteSize      int64
	PhotoUploadPartSize   int64
	PhotoUploadMaxParts   int32
	PhotoUploadSignTTL    time.Duration
	PhotoUploadSessionTTL time.Duration
	FaceAIURL             string
	DependencyTimeout     time.Duration
	AuthSigningKey        string
	AuthIssuer            string
	AuthAudience          string
	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	RefreshCookieSecure   bool
	WebOrigin             string
	OutboxStreamName      string
	OutboxPollInterval    time.Duration
	OutboxBatchSize       int
	OutboxLeaseTTL        time.Duration
}

func Load() Config {
	return Config{
		Host:                  valueOrDefault("API_HOST", "0.0.0.0"),
		Port:                  valueOrDefault("API_PORT", "8080"),
		DatabaseURL:           valueOrDefault("DATABASE_URL", "postgres://face_search:face_search@localhost:5432/face_search?sslmode=disable"),
		DatabaseMaxConns:      int32Value("DATABASE_MAX_CONNECTIONS", 10),
		SchemaVersion:         int64Value("DATABASE_SCHEMA_VERSION", 3),
		RedisURL:              valueOrDefault("REDIS_URL", "redis://localhost:6379/0"),
		QdrantURL:             valueOrDefault("QDRANT_URL", "http://localhost:6333"),
		MinIOEndpoint:         valueOrDefault("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:        valueOrDefault("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:        valueOrDefault("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseTLS:           boolValue("MINIO_USE_TLS", false),
		MinIOBucket:           valueOrDefault("MINIO_BUCKET", "face-search"),
		PhotoMaxByteSize:      boundedInt64Value("PHOTO_MAX_BYTE_SIZE", 100*1024*1024, 1, 5*1024*1024*1024),
		PhotoUploadPartSize:   boundedInt64Value("PHOTO_UPLOAD_PART_SIZE", 8*1024*1024, 5*1024*1024, 5*1024*1024*1024),
		PhotoUploadMaxParts:   int32(boundedInt64Value("PHOTO_UPLOAD_MAX_PARTS", 1000, 1, 10000)),
		PhotoUploadSignTTL:    boundedDurationValue("PHOTO_UPLOAD_SIGN_TTL", 10*time.Minute, time.Minute, 24*time.Hour),
		PhotoUploadSessionTTL: boundedDurationValue("PHOTO_UPLOAD_SESSION_TTL", 24*time.Hour, time.Minute, 7*24*time.Hour),
		FaceAIURL:             valueOrDefault("FACE_AI_URL", "http://localhost:8001"),
		DependencyTimeout:     durationValue("DEPENDENCY_TIMEOUT", 3*time.Second),
		AuthSigningKey:        os.Getenv("AUTH_SIGNING_KEY"),
		AuthIssuer:            valueOrDefault("AUTH_ISSUER", "face-search-api"),
		AuthAudience:          valueOrDefault("AUTH_AUDIENCE", "face-search-web"),
		AccessTokenTTL:        durationValue("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:       durationValue("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		RefreshCookieSecure:   boolValue("REFRESH_COOKIE_SECURE", false),
		WebOrigin:             valueOrDefault("WEB_ORIGIN", "http://localhost:3000"),
		OutboxStreamName:      valueOrDefault("REDIS_STREAM", "photo-jobs"),
		OutboxPollInterval:    boundedDurationValue("OUTBOX_POLL_INTERVAL", 2*time.Second, 500*time.Millisecond, time.Minute),
		OutboxBatchSize:       int(boundedInt64Value("OUTBOX_BATCH_SIZE", 50, 1, 500)),
		OutboxLeaseTTL:        boundedDurationValue("OUTBOX_LEASE_TTL", 30*time.Second, 5*time.Second, 5*time.Minute),
	}
}

func (c Config) Address() string { return c.Host + ":" + c.Port }

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolValue(key string, fallback bool) bool {
	value, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func int32Value(key string, fallback int32) int32 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 32)
	if err != nil || value <= 0 {
		return fallback
	}
	return int32(value)
}

func int64Value(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boundedInt64Value(key string, fallback, minimum, maximum int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func boundedDurationValue(key string, fallback, minimum, maximum time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func durationValue(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
