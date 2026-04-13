package logger

import (
	"context"
	"log/slog"
)

type ctxKey string

const loggerKey ctxKey = "logger"

func WithContext(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return L()
}
