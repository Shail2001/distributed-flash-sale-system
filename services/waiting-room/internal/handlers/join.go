package handlers

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type joinRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type queueResponse struct {
	UserID        string `json:"user_id"`
	Position      int64  `json:"position"`
	AdmittedCount int64  `json:"admitted_count"`
	Admitted      bool   `json:"admitted"`
	Token         string `json:"token,omitempty"`
	Strategy      string `json:"strategy"`
}

// Join handles POST /queue/join.
// Assigns a queue position and returns an admission token immediately if already admitted.
// Idempotent: calling twice for the same user_id returns the same position.
// Out-of-order detection: if position != sizeBefore+1, the timestamp-only strategy
// produced a non-sequential assignment — counted for Experiment 1.
func (h *Handler) Join(c *gin.Context) {
	var req joinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Record total size before this join for out-of-order detection.
	sizeBefore, _ := h.store.QueueSize(ctx, h.itemID)

	position, err := h.store.Join(ctx, req.UserID, h.itemID, h.strategy)
	if err != nil {
		slog.Error("queue join error", slog.String("user_id", req.UserID), slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue error"})
		return
	}

	h.metrics.IncJoins()

	// Out-of-order: position should be sizeBefore+1 for perfect sequential assignment.
	// Any deviation means a timestamp collision resolved non-deterministically.
	if sizeBefore > 0 && position != sizeBefore+1 {
		h.metrics.IncOutOfOrder()
	}

	_, admittedCount, err := h.store.GetPosition(ctx, req.UserID, h.itemID)
	if err != nil {
		slog.Error("get position after join", slog.String("user_id", req.UserID), slog.Any("error", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue error"})
		return
	}

	resp := queueResponse{
		UserID:        req.UserID,
		Position:      position,
		AdmittedCount: admittedCount,
		Strategy:      h.strategy,
	}

	if position <= admittedCount {
		token, err := h.admissionToken(ctx, req.UserID)
		if err != nil {
			slog.Error("token error on join", slog.String("user_id", req.UserID), slog.Any("error", err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token error"})
			return
		}
		resp.Admitted = true
		resp.Token = token
		h.metrics.IncAdmitted()
	}

	slog.Info("user joined queue",
		slog.String("user_id", req.UserID),
		slog.Int64("position", position),
		slog.Bool("admitted", resp.Admitted),
		slog.String("strategy", h.strategy),
	)

	c.JSON(http.StatusCreated, resp)
}
