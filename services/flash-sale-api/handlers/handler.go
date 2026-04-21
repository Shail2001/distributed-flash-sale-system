package handlers

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// OrderPublisher captures the publish contract needed by the purchase handler.
type OrderPublisher interface {
	PublishOrder(ctx context.Context, orderID, customerID, itemID string, quantity int) error
}

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	rdb            *redis.Client
	publisher      OrderPublisher
	inventoryCount int
	inventoryMode  string
}

// NewHandler constructs a Handler with injected dependencies.
func NewHandler(rdb *redis.Client, publisher OrderPublisher, inventoryCount int, inventoryMode string) *Handler {
	return &Handler{
		rdb:            rdb,
		publisher:      publisher,
		inventoryCount: inventoryCount,
		inventoryMode:  inventoryMode,
	}
}
