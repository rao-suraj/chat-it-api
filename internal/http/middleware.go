package http

import (
	"chat-it-api/internal/errors"
	"chat-it-api/internal/logger"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqID := "req_" + uuid.NewString()

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

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		err := c.Errors.Last().Err
		log := logger.FromContext(c.Request.Context())

		appErr, ok := err.(*errors.AppError)
		if !ok {
			// unexpected error — log with stack trace
			log.Error("unexpected error",
				slog.String("error", err.Error()),
				slog.String("stack", fmt.Sprintf("%+v", err)),
			)
			appErr = errors.Internal()
		} else if appErr.Status >= 500 {
			log.Error("internal app error",
				slog.String("code", appErr.Code),
				slog.String("error", appErr.Message),
				slog.String("stack", fmt.Sprintf("%+v", err)),
			)
		} else {
			// expected client errors — no stack needed
			log.Warn("client error",
				slog.String("code", appErr.Code),
				slog.String("error", appErr.Message),
			)
		}

		c.JSON(appErr.Status, gin.H{
			"request_id": c.GetString("request_id"),
			"error": gin.H{
				"code":    appErr.Code,
				"message": appErr.Message,
			},
		})
	}
}

func Recovery() gin.HandlerFunc {
	return gin.RecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, err any) {
		log := logger.FromContext(c.Request.Context())
		log.Error("panic recovered",
			slog.String("error", fmt.Sprintf("%v", err)),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"request_id": c.GetString("request_id"),
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "something went wrong",
			},
		})
	})
}