package logger

import (
	"log/slog"
	"os"
	"sync"
)

type Config struct {
	Level  string // "debug", "info"
	Format string // "json", "text"
}

var (
	global *slog.Logger
	once   sync.Once
)

func Init(cfg Config) {
	once.Do(func() {
		level := slog.LevelInfo
		if cfg.Level == "debug" {
			level = slog.LevelDebug
		}

		opts := &slog.HandlerOptions{Level: level}

		var handler slog.Handler
		if cfg.Format == "json" {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(os.Stdout, opts)
		}

		global = slog.New(handler).With(
			slog.String("service", "chat-it-api"),
		)
	})
}

func L() *slog.Logger {
	if global == nil {
		panic("logger not initialized — call logger.Init() first")
	}
	return global
}
