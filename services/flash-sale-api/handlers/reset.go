package handlers

import (
	"log"
	"net/http"

	redisclient "flash-sale-api/redis"

	"github.com/gin-gonic/gin"
)

type resetResponse struct {
	Status    string `json:"status"`
	Remaining int    `json:"remaining"`
}

// Reset handles POST /reset.
// Restores the Redis inventory counter to INVENTORY_COUNT.
// Use between experiments to start from a clean slate.
func (h *Handler) Reset(c *gin.Context) {
	ctx := c.Request.Context()
	const itemID = "flash-sale-item"

	if err := redisclient.SetInventory(ctx, h.rdb, itemID, h.inventoryCount); err != nil {
		log.Printf("Redis SET (reset) error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset failed"})
		return
	}

	log.Printf("Inventory reset to %d", h.inventoryCount)

	c.JSON(http.StatusOK, resetResponse{
		Status:    "reset",
		Remaining: h.inventoryCount,
	})
}
