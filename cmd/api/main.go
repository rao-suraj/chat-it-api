package main

import (
	"chat-it-api/internal/config"
	apphttp "chat-it-api/internal/http"
	"chat-it-api/internal/logger"
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	logger.Init(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      apphttp.NewRouter(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.L().Info("server starting", slog.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L().Error("server failed to start", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.L().Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.L().Error("server forced to shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.L().Info("server stopped")
}
