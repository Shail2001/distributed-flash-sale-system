package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Reset handles POST /queue/reset.
// Clears all queue state for this item — use between experiment runs.
func (h *Handler) Reset(c *gin.Context) {
	if err := h.store.Reset(c.Request.Context(), h.itemID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reset", "item_id": h.itemID})
}
