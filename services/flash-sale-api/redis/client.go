package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient creates and returns a configured Redis client.
func NewClient(endpoint, port string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", endpoint, port),
	})
}

const (
	StrategyAtomicDecr = "atomic_decr"
	StrategyOptimistic = "optimistic"
	StrategyLuaScript  = "lua_script"
)

var (
	ErrSoldOut         = errors.New("sold out")
	ErrUnknownStrategy = errors.New("unknown inventory strategy")
)

var reserveLuaScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local qty = tonumber(ARGV[1])
if current < qty then
  return -1
end
return redis.call("DECRBY", KEYS[1], qty)
`)

// DecrInventory atomically decrements the inventory counter for the given itemID.
// Returns the new value after decrement.
// Caller must check: if result < 0, sold out → INCR back and return 409.
func DecrInventory(ctx context.Context, rdb *redis.Client, itemID string) (int64, error) {
	return DecrInventoryBy(ctx, rdb, itemID, 1)
}

// DecrInventoryBy atomically decrements the inventory counter by quantity.
func DecrInventoryBy(ctx context.Context, rdb *redis.Client, itemID string, quantity int) (int64, error) {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.DecrBy(ctx, key, int64(quantity)).Result()
}

// ReserveInventory attempts to reserve quantity for an item based on strategy.
// Returns remaining inventory on success or ErrSoldOut when insufficient.
func ReserveInventory(ctx context.Context, rdb *redis.Client, itemID string, quantity int, strategy string) (int64, error) {
	switch strategy {
	case "", StrategyAtomicDecr:
		remaining, err := DecrInventoryBy(ctx, rdb, itemID, quantity)
		if err != nil {
			return 0, err
		}
		if remaining < 0 {
			if incrErr := IncrInventoryBy(ctx, rdb, itemID, quantity); incrErr != nil {
				return 0, fmt.Errorf("sold out correction failed: %w", incrErr)
			}
			return 0, ErrSoldOut
		}
		return remaining, nil
	case StrategyOptimistic:
		return reserveWithOptimisticLock(ctx, rdb, itemID, quantity)
	case StrategyLuaScript:
		return reserveWithLua(ctx, rdb, itemID, quantity)
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnknownStrategy, strategy)
	}
}

func reserveWithOptimisticLock(ctx context.Context, rdb *redis.Client, itemID string, quantity int) (int64, error) {
	key := fmt.Sprintf("inventory:%s", itemID)
	for attempts := 0; attempts < 10; attempts++ {
		var remaining int64
		err := rdb.Watch(ctx, func(tx *redis.Tx) error {
			current, err := tx.Get(ctx, key).Int64()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					current = 0
				} else {
					return err
				}
			}
			if current < int64(quantity) {
				return ErrSoldOut
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				remaining = current - int64(quantity)
				pipe.DecrBy(ctx, key, int64(quantity))
				return nil
			})
			return err
		}, key)
		if err == nil {
			return remaining, nil
		}
		if errors.Is(err, ErrSoldOut) {
			return 0, ErrSoldOut
		}
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return 0, err
	}
	return 0, redis.TxFailedErr
}

func reserveWithLua(ctx context.Context, rdb *redis.Client, itemID string, quantity int) (int64, error) {
	key := fmt.Sprintf("inventory:%s", itemID)
	result, err := reserveLuaScript.Run(ctx, rdb, []string{key}, quantity).Result()
	if err != nil {
		return 0, err
	}
	remaining, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("unexpected lua result type %T", result)
	}
	if remaining < 0 {
		return 0, ErrSoldOut
	}
	return remaining, nil
}

// IncrInventory increments the counter back — used to correct a negative value on sold-out.
func IncrInventory(ctx context.Context, rdb *redis.Client, itemID string) error {
	return IncrInventoryBy(ctx, rdb, itemID, 1)
}

// IncrInventoryBy increments the counter back by quantity after a failed attempt.
func IncrInventoryBy(ctx context.Context, rdb *redis.Client, itemID string, quantity int) error {
	key := fmt.Sprintf("inventory:%s", itemID)
	return rdb.IncrBy(ctx, key, int64(quantity)).Err()
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
