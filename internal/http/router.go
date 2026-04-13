package http

import (
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestLogger())

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Chatbot API ready!"})
	})

	return r
}
