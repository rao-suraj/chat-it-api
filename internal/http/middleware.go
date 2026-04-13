package http

import (
	"chat-it-api/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"log/slog"
	"time"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqID := uuid.NewString()

		log := logger.L().With(
			slog.String("request_id", reqID),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
		)

		c.Set("request_id", reqID)
		c.Request = c.Request.WithContext(logger.WithContext(c.Request.Context(), log))

		c.Next()

		log.Info("http_request_completed",
			slog.Int("status", c.Writer.Status()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	}
}