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
	server := &http.Server{Addr: cfg.Address(), Handler: httpserver.NewRouterWithAuth(dependencies, authHandler, cfg.WebOrigin), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("api listening", "address", cfg.Address())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api stopped unexpectedly", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown failed", "error", err)
	}
}
