package platform

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/face-search-ai/api/internal/config"
	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/store"
	"github.com/face-search-ai/api/internal/store/postgres"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

type Dependencies struct {
	postgres      *postgres.Store
	redis         *redis.Client
	minio         *minio.Client
	cfg           config.Config
	httpClient    *http.Client
	auth          *auth.Service
	authorization *authorization.Service
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
	return &Dependencies{
		postgres:   pool,
		redis:      redis.NewClient(redisOptions),
		minio:      minioClient,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.DependencyTimeout},
		auth:       authService,
		authorization: authorization.NewServiceWithAuditor(
			postgres.NewAuthorizationRepository(pool),
			postgres.NewAuditRepository(pool),
		),
	}, nil
}

func (d *Dependencies) AuthService() *auth.Service                   { return d.auth }
func (d *Dependencies) AuthorizationService() *authorization.Service { return d.authorization }
func (d *Dependencies) Config() config.Config                        { return d.cfg }

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
			err := check(checkCtx)
			status := Status{OK: err == nil}
			if err != nil {
				status.Error = err.Error()
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
