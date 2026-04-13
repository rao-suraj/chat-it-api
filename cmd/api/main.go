package main

import (
	"chat-it-api/internal/config"
	"chat-it-api/internal/http"
	"chat-it-api/internal/logger"
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

	r := http.NewRouter()
	r.Run(":" + cfg.Port)
}
