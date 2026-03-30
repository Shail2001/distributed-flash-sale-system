package handlers

import (
	"log"
	"net/http"

	redisclient "flash-sale-api/redis"

	"github.com/gin-gonic/gin"
)

type inventoryResponse struct {
	ItemID    string `json:"item_id"`
	Remaining int64  `json:"remaining"`
	SoldOut   bool   `json:"sold_out"`
}

type healthResponse struct {
	Status    string `json:"status"`
	Remaining int64  `json:"remaining"`
}

// GetInventory handles GET /inventory.
// Reads the Redis counter — does NOT decrement.
func (h *Handler) GetInventory(c *gin.Context) {
	ctx := c.Request.Context()
	const itemID = "flash-sale-item"

	remaining, err := redisclient.GetInventory(ctx, h.rdb, itemID)
	if err != nil {
		log.Printf("Redis GET inventory error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error"})
		return
	}

	// Clamp to 0 for display — counter can go briefly negative during a race before correction.
	if remaining < 0 {
		remaining = 0
	}

	c.JSON(http.StatusOK, inventoryResponse{
		ItemID:    itemID,
		Remaining: remaining,
		SoldOut:   remaining == 0,
	})
}

// Health handles GET /health.
// Pings Redis and returns inventory remaining — a failed ping returns 503.
func (h *Handler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	const itemID = "flash-sale-item"

	remaining, err := redisclient.GetInventory(ctx, h.rdb, itemID)
	if err != nil {
		log.Printf("Health check Redis error: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "redis unreachable",
		})
		return
	}

	if remaining < 0 {
		remaining = 0
	}

	c.JSON(http.StatusOK, healthResponse{
		Status:    "healthy",
		Remaining: remaining,
	})
}
