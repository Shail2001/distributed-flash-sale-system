package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient creates and returns a configured Redis client.
func NewClient(endpoint, port string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", endpoint, port),
	})
}

// DecrInventory atomically decrements the inventory counter for the given itemID.
// Returns the new value after decrement.
// Caller must check: if result < 0, sold out → INCR back and return 409.
func DecrInventory(ctx context.Context, rdb *redis.Client, itemID string) (int64, error) {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.Decr(ctx, key).Result()
}

// IncrInventory increments the counter back — used to correct a negative value on sold-out.
func IncrInventory(ctx context.Context, rdb *redis.Client, itemID string) error {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.Incr(ctx, key).Err()
}

// GetInventory reads the current counter without modifying it.
func GetInventory(ctx context.Context, rdb *redis.Client, itemID string) (int64, error) {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.Get(ctx, key).Int64()
}

// SetInventory hard-sets the counter — used by the reset endpoint.
func SetInventory(ctx context.Context, rdb *redis.Client, itemID string, count int) error {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.Set(ctx, key, count, 0).Err()
}
