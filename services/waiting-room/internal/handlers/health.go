package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health handles GET /health.
// Pings Redis and returns 200 if healthy, 503 if Redis is unreachable.
func (h *Handler) Health(c *gin.Context) {
	if err := h.store.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "redis unreachable",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
