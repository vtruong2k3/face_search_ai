package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host              string
	Port              string
	DatabaseURL       string
	RedisURL          string
	QdrantURL         string
	MinIOEndpoint     string
	MinIOAccessKey    string
	MinIOSecretKey    string
	MinIOUseTLS       bool
	MinIOBucket       string
	FaceAIURL         string
	DependencyTimeout time.Duration
}

func Load() Config {
	return Config{
		Host:              valueOrDefault("API_HOST", "0.0.0.0"),
		Port:              valueOrDefault("API_PORT", "8080"),
		DatabaseURL:       valueOrDefault("DATABASE_URL", "postgres://face_search:face_search@localhost:5432/face_search?sslmode=disable"),
		RedisURL:          valueOrDefault("REDIS_URL", "redis://localhost:6379/0"),
		QdrantURL:         valueOrDefault("QDRANT_URL", "http://localhost:6333"),
		MinIOEndpoint:     valueOrDefault("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:    valueOrDefault("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:    valueOrDefault("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseTLS:       boolValue("MINIO_USE_TLS", false),
		MinIOBucket:       valueOrDefault("MINIO_BUCKET", "face-search"),
		FaceAIURL:         valueOrDefault("FACE_AI_URL", "http://localhost:8001"),
		DependencyTimeout: durationValue("DEPENDENCY_TIMEOUT", 3*time.Second),
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

func durationValue(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
