package http

import (
	v1 "chat-it-api/internal/http/handlers/v1"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(Recovery())
	r.Use(RequestLogger())
	r.Use(ErrorHandler())

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "the requested route does not exist",
			},
		})
	})

	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error": gin.H{
				"code":    "METHOD_NOT_ALLOWED",
				"message": "method not allowed",
			},
		})
	})

	api := r.Group("/api")
	{
		v1Group := api.Group("/v1")
		{
			v1.RegisterHealthRoutes(v1Group)
			v1.RegisterChatRoutes(v1Group)
		}
	}

	return r
}