package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/face-search-ai/api/internal/config"
	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/domain/event"
	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/face-search-ai/api/internal/domain/search"
	"github.com/face-search-ai/api/internal/downloadinfra"
	"github.com/face-search-ai/api/internal/observability"
	"github.com/face-search-ai/api/internal/ratelimit"
	"github.com/face-search-ai/api/internal/searchinfra"
	"github.com/face-search-ai/api/internal/storage/objectstorage"
	"github.com/face-search-ai/api/internal/store"
	"github.com/face-search-ai/api/internal/store/postgres"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

type Dependencies struct {
	postgres        *postgres.Store
	redis           *redis.Client
	minio           *minio.Client
	cfg             config.Config
	httpClient      *http.Client
	auth            *auth.Service
	authorization   *authorization.Service
	events          *event.Service
	photos          *photo.Service
	photoUploads    *photo.UploadService
	outboxPublisher *outboxPublisher
	search          *search.Service
	downloads       *download.Service
	downloadLimiter *ratelimit.Limiter
	authLimiter     *ratelimit.Limiter
	searchLimiter   *ratelimit.Limiter
}

type Status struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func New(ctx context.Context, cfg config.Config) (*Dependencies, error) {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns, cfg.SchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("postgres config: %w", err)
	}
	authService, err := auth.NewService(postgres.NewAuthRepository(pool), cfg.AuthSigningKey, cfg.AuthIssuer, cfg.AuthAudience, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("auth config: %w", err)
	}
	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("redis config: %w", err)
	}
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""), Secure: cfg.MinIOUseTLS})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("minio config: %w", err)
	}
	uploadPolicy, err := photo.NewUploadPolicy(cfg.PhotoMaxByteSize, cfg.PhotoUploadPartSize, int(cfg.PhotoUploadMaxParts), cfg.PhotoUploadSignTTL, cfg.PhotoUploadSessionTTL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("photo upload config: %w", err)
	}
	photoRepository := postgres.NewPhotoRepository(pool)
	redisClient := redis.NewClient(redisOptions)
	apiHTTPClient := &http.Client{Timeout: cfg.DependencyTimeout}
	faceAIClient, err := searchinfra.NewFaceAIClient(cfg.FaceAIURL, cfg.FaceAIInternalToken, apiHTTPClient)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("face ai config: %w", err)
	}
	qdrantClient, err := searchinfra.NewQdrantClient(cfg.QdrantURL, cfg.QdrantCollection, apiHTTPClient)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("qdrant config: %w", err)
	}
	eventService := event.NewService(postgres.NewEventRepository(pool))
	searchService, err := search.NewService(searchinfra.NewScopeResolver(eventService), faceAIClient, qdrantClient, cfg.SearchThreshold, cfg.SearchResultLimit)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("search config: %w", err)
	}
	downloadRepository := postgres.NewDownloadRepository(pool)
	downloadService, err := download.NewService(
		downloadinfra.NewScopeResolver(eventService),
		downloadRepository,
		objectstorage.NewMinIO(minioClient, cfg.MinIOBucket),
		downloadRepository,
		cfg.DownloadURLTTL,
		cfg.DownloadMaxBulk,
	)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("download config: %w", err)
	}
	outboxRepo := postgres.NewOutboxRepository(pool)
	outboxCfg := OutboxPublisherConfig{
		StreamName:   cfg.OutboxStreamName,
		PollInterval: cfg.OutboxPollInterval,
		BatchSize:    cfg.OutboxBatchSize,
		LeaseTTL:     cfg.OutboxLeaseTTL,
	}
	return &Dependencies{
		postgres:   pool,
		redis:      redisClient,
		minio:      minioClient,
		cfg:        cfg,
		httpClient: apiHTTPClient,
		auth:       authService,
		authorization: authorization.NewServiceWithAuditor(
			postgres.NewAuthorizationRepository(pool),
			postgres.NewAuditRepository(pool),
		),
		events:          event.NewService(postgres.NewEventRepository(pool)),
		photos:          photo.NewService(photoRepository),
		photoUploads:    photo.NewUploadService(photoRepository, postgres.NewPhotoUploadRepository(pool), objectstorage.NewMinIO(minioClient, cfg.MinIOBucket), uploadPolicy),
		outboxPublisher: newOutboxPublisher(outboxRepo, redisClient, outboxCfg),
		search:          searchService,
		downloads:       downloadService,
		downloadLimiter: ratelimit.New(cfg.DownloadRateLimit, cfg.DownloadRateWindow),
		authLimiter:     ratelimit.New(cfg.AuthRateLimit, cfg.AuthRateWindow),
		searchLimiter:   ratelimit.New(cfg.SearchRateLimit, cfg.SearchRateWindow),
	}, nil
}

func (d *Dependencies) AuthService() *auth.Service                   { return d.auth }
func (d *Dependencies) AuthorizationService() *authorization.Service { return d.authorization }
func (d *Dependencies) EventService() *event.Service                 { return d.events }
func (d *Dependencies) PhotoService() *photo.Service                 { return d.photos }
func (d *Dependencies) PhotoUploadService() *photo.UploadService     { return d.photoUploads }
func (d *Dependencies) SearchService() *search.Service               { return d.search }
func (d *Dependencies) DownloadService() *download.Service           { return d.downloads }
func (d *Dependencies) DownloadLimiter() *ratelimit.Limiter          { return d.downloadLimiter }
func (d *Dependencies) AuthLimiter() *ratelimit.Limiter              { return d.authLimiter }
func (d *Dependencies) SearchLimiter() *ratelimit.Limiter            { return d.searchLimiter }
func (d *Dependencies) Config() config.Config                        { return d.cfg }

// RunOutboxPublisher starts the outbox polling loop. Call from main in a goroutine;
// it blocks until ctx is cancelled.
func (d *Dependencies) RunOutboxPublisher(ctx context.Context) {
	d.outboxPublisher.Run(ctx)
}

func (d *Dependencies) Persistence() interface {
	store.DBTX
	store.Transactor
	store.SchemaChecker
} {
	return d.postgres
}

func (d *Dependencies) Close() { d.redis.Close(); d.postgres.Close() }

func (d *Dependencies) Check(ctx context.Context) map[string]Status {
	checks := map[string]func(context.Context) error{
		"postgres": func(ctx context.Context) error {
			if err := d.postgres.Ping(ctx); err != nil {
				return err
			}
			return d.postgres.CheckSchema(ctx)
		},
		"redis":  func(ctx context.Context) error { return d.redis.Ping(ctx).Err() },
		"minio":  func(ctx context.Context) error { _, err := d.minio.ListBuckets(ctx); return err },
		"qdrant": func(ctx context.Context) error { return d.get(ctx, strings.TrimRight(d.cfg.QdrantURL, "/")+"/healthz") },
		"face_ai": func(ctx context.Context) error {
			return d.get(ctx, strings.TrimRight(d.cfg.FaceAIURL, "/")+"/health/ready")
		},
	}
	result := make(map[string]Status, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, check := range checks {
		name, check := name, check
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, d.cfg.DependencyTimeout)
			defer cancel()
			started := time.Now()
			err := check(checkCtx)
			latency := time.Since(started)

			// Emit sanitized, actionable telemetry: the fixed dependency name, a
			// healthy/unhealthy result, and a latency class gauge/histogram. The raw
			// error is deliberately never exported as a metric label/value or logged,
			// because it can contain a connection string, URL, or credential.
			observability.RecordDependencyCheck(name, err == nil, latency)

			status := Status{OK: err == nil}
			if err != nil {
				// The readiness endpoint is reachable through the edge proxy, so the
				// per-dependency detail is reduced to a uniform, non-leaking token. The
				// dependency name (the map key) already tells operators which dependency
				// is unhealthy, which is the actionable part.
				status.Error = "unavailable"
				slog.Warn("dependency unhealthy", "dependency", name, "latency_ms", latency.Milliseconds())
			}
			mu.Lock()
			result[name] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

func (d *Dependencies) get(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func Ready(statuses map[string]Status) bool {
	for _, status := range statuses {
		if !status.OK {
			return false
		}
	}
	return true
}
