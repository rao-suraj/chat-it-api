package v1

import (
	"chat-it-api/internal/errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterChatRoutes(r *gin.RouterGroup) {
	r.POST("/chat", chat)
}

func chat(c *gin.Context) {
	var req struct {
		Message string `json:"message"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.BadRequest("INVALID_BODY", "invalid request body"))
		return
	}

	if req.Message == "" {
		c.Error(errors.BadRequest("MISSING_FIELD", "message is required"))
		return
	}

	// call service here later
	c.JSON(http.StatusOK, gin.H{"reply": "received: " + req.Message})
}