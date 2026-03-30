package handlers

import (
	sqsclient "flash-sale-api/sqs"

	"github.com/redis/go-redis/v9"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	rdb            *redis.Client
	publisher      *sqsclient.Publisher
	inventoryCount int
}

// NewHandler constructs a Handler with injected dependencies.
func NewHandler(rdb *redis.Client, publisher *sqsclient.Publisher, inventoryCount int) *Handler {
	return &Handler{
		rdb:            rdb,
		publisher:      publisher,
		inventoryCount: inventoryCount,
	}
}
