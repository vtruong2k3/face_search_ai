package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/face-search-ai/api/internal/config"
	httpserver "github.com/face-search-ai/api/internal/http"
	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/platform"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	dependencies, err := platform.New(ctx, cfg)
	if err != nil {
		slog.Error("dependency initialization failed", "error", err)
		os.Exit(1)
	}
	defer dependencies.Close()
	authHandler := handlers.NewAuth(dependencies.AuthService(), cfg.RefreshCookieSecure, cfg.RefreshTokenTTL)
	organizationsHandler := handlers.NewOrganizations(dependencies.AuthorizationService())
	eventsHandler := handlers.NewEvents(dependencies.EventService(), dependencies.AuthorizationService())
	photosHandler := handlers.NewPhotos(dependencies.PhotoService(), dependencies.PhotoUploadService(), dependencies.AuthorizationService())
	searchHandler := handlers.NewSearch(dependencies.SearchService())
	downloadsHandler := handlers.NewDownloads(dependencies.DownloadService(), dependencies.DownloadLimiter(), dependencies.AuthorizationService())
	server := &http.Server{
		Addr: cfg.Address(),
		Handler: httpserver.NewRouterWithAuth(
			dependencies,
			authHandler,
			dependencies.AuthService(),
			organizationsHandler,
			eventsHandler,
			photosHandler,
			searchHandler,
			downloadsHandler,
			httpserver.SecurityControls{
				WebOrigin:      cfg.WebOrigin,
				RequestTimeout: cfg.HTTPRequestTimeout,
				AuthLimiter:    dependencies.AuthLimiter(),
				SearchLimiter:  dependencies.SearchLimiter(),
			},
		),
		// ReadHeaderTimeout caps slow header attacks; ReadTimeout bounds the whole
		// request body (the largest legitimate API body is the ~10 MiB selfie search);
		// WriteTimeout is set above the per-request handler timeout so the handler can
		// emit a safe timeout response before the connection is force-closed; IdleTimeout
		// reaps idle keep-alive connections.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}
	go func() {
		slog.Info("api listening", "address", cfg.Address())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api stopped unexpectedly", "error", err)
			stop()
		}
	}()
	go dependencies.RunOutboxPublisher(ctx)
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown failed", "error", err)
	}
}
