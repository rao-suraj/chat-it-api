package v1

import (
	"chat-it-api/internal/response"

	"github.com/gin-gonic/gin"
)

func RegisterHealthRoutes(r *gin.RouterGroup) {
	r.GET("/health", health)
}

func health(c *gin.Context) {
	response.OK(c, gin.H{"status": "ok"})
}