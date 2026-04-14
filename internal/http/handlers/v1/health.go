package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterHealthRoutes(r *gin.RouterGroup) {
	r.GET("/health", health)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}