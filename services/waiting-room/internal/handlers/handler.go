package handlers

import (
	"context"

	"waiting-room/internal/metrics"
	"waiting-room/internal/queue"

	"github.com/google/uuid"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	store    *queue.Store
	strategy string
	itemID   string
	metrics  *metrics.Collector
}

func NewHandler(store *queue.Store, strategy, itemID string, m *metrics.Collector) *Handler {
	return &Handler{store: store, strategy: strategy, itemID: itemID, metrics: m}
}

// admissionToken returns an existing token for userID, or generates and stores a new one.
// SetNX in IssueToken ensures only one token wins in concurrent multi-replica scenarios.
func (h *Handler) admissionToken(ctx context.Context, userID string) (string, error) {
	// Return existing token if already issued.
	if existing, err := h.store.GetToken(ctx, userID); err == nil && existing != "" {
		return existing, nil
	}

	token := uuid.New().String()
	// SetNX: safe to call from multiple replicas simultaneously.
	if err := h.store.IssueToken(ctx, userID, token); err != nil {
		return "", err
	}

	// Re-fetch: another replica may have won the SetNX race.
	// Either way, the stored value is a valid token.
	if final, err := h.store.GetToken(ctx, userID); err == nil && final != "" {
		return final, nil
	}

	return token, nil
}
