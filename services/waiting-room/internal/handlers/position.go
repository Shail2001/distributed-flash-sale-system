package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Position handles GET /queue/position?user_id=xxx
// Clients poll this endpoint. When admitted=true, the response includes a token
// to present to the Flash Sale API.
func (h *Handler) Position(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id query parameter is required"})
		return
	}

	ctx := c.Request.Context()
	h.metrics.IncPollsServed()

	position, admittedCount, err := h.store.GetPosition(ctx, userID, h.itemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue error"})
		return
	}

	if position == -1 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NOT_IN_QUEUE",
			"message": "user_id not found in queue — call POST /queue/join first",
		})
		return
	}

	resp := queueResponse{
		UserID:        userID,
		Position:      position,
		AdmittedCount: admittedCount,
		Strategy:      h.strategy,
	}

	if position <= admittedCount {
		token, err := h.admissionToken(ctx, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
			return
		}
		resp.Admitted = true
		resp.Token = token
		h.metrics.IncAdmitted()
	}

	c.JSON(http.StatusOK, resp)
}
