package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	tokenTTL = 10 * time.Minute

	prefixQueue    = "queue:"
	prefixAdmitted = "admitted:"
	prefixSeq      = "seq:"
	prefixToken    = "token:"
)

// Store wraps Redis operations for the waiting room queue.
type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

// Join adds userID to the queue for itemID using the given strategy.
// Idempotent: if userID is already present, returns their existing position unchanged.
// Returns 1-indexed position.
func (s *Store) Join(ctx context.Context, userID, itemID, strategy string) (int64, error) {
	queueKey := prefixQueue + itemID

	// Already in queue — return existing rank without changing it.
	if rank, err := s.rdb.ZRank(ctx, queueKey, userID).Result(); err == nil {
		return rank + 1, nil
	}

	score := s.scoreFor(ctx, itemID, strategy)

	if err := s.rdb.ZAdd(ctx, queueKey, redis.Z{Score: score, Member: userID}).Err(); err != nil {
		return 0, fmt.Errorf("zadd: %w", err)
	}

	rank, err := s.rdb.ZRank(ctx, queueKey, userID).Result()
	if err != nil {
		return 0, fmt.Errorf("zrank after join: %w", err)
	}

	return rank + 1, nil
}

// GetPosition returns the 1-indexed position and current admitted count for userID.
// Returns position -1 if userID is not in the queue.
func (s *Store) GetPosition(ctx context.Context, userID, itemID string) (position, admittedCount int64, err error) {
	queueKey := prefixQueue + itemID
	admittedKey := prefixAdmitted + itemID

	rank, rankErr := s.rdb.ZRank(ctx, queueKey, userID).Result()
	if rankErr == redis.Nil {
		return -1, 0, nil
	}
	if rankErr != nil {
		return 0, 0, fmt.Errorf("zrank: %w", rankErr)
	}

	admitted, admErr := s.rdb.Get(ctx, admittedKey).Int64()
	if admErr == redis.Nil {
		admitted = 0
	} else if admErr != nil {
		return 0, 0, fmt.Errorf("get admitted: %w", admErr)
	}

	return rank + 1, admitted, nil
}

// IssueToken stores token for userID with SetNX — safe for concurrent calls across replicas.
func (s *Store) IssueToken(ctx context.Context, userID, token string) error {
	return s.rdb.SetNX(ctx, prefixToken+userID, token, tokenTTL).Err()
}

// GetToken retrieves an existing admission token for userID. Returns "" if none.
func (s *Store) GetToken(ctx context.Context, userID string) (string, error) {
	token, err := s.rdb.Get(ctx, prefixToken+userID).Result()
	if err == redis.Nil {
		return "", nil
	}
	return token, err
}

// IncrAdmitted increments the admitted counter for itemID by n.
// Called by the admission ticker once per tick.
func (s *Store) IncrAdmitted(ctx context.Context, itemID string, n int64) error {
	return s.rdb.IncrBy(ctx, prefixAdmitted+itemID, n).Err()
}

// Reset clears all queue state for itemID.
// Use between experiment runs to start from a clean slate.
func (s *Store) Reset(ctx context.Context, itemID string) error {
	keys := []string{
		prefixQueue + itemID,
		prefixAdmitted + itemID,
		prefixSeq + itemID,
	}
	return s.rdb.Del(ctx, keys...).Err()
}

// Ping checks Redis connectivity — used by the health endpoint.
func (s *Store) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

// scoreFor returns the ZADD score based on the strategy.
//
// "timestamp"       — millisecond Unix timestamp.
//                     Ties are possible when two instances call within the same ms.
//                     This is Strategy A for Experiment 1: measures out-of-order rate.
//
// "timestamp_incr"  — millisecond timestamp + Redis INCR tiebreaker as fractional part.
//                     The INCR is global and atomic, so no two members share a score.
//                     This is Strategy B for Experiment 1: eliminates out-of-order assignments.
func (s *Store) scoreFor(ctx context.Context, itemID, strategy string) float64 {
	ms := float64(time.Now().UnixMilli())
	if strategy != "timestamp_incr" {
		return ms
	}
	seq, err := s.rdb.Incr(ctx, prefixSeq+itemID).Result()
	if err != nil {
		// Degrade gracefully — fall back to timestamp-only on INCR failure.
		return ms
	}
	// seq/1e9 is sub-millisecond and does not affect ordering relative to other ms buckets,
	// but breaks ties deterministically within the same millisecond.
	return ms + float64(seq)/1e9
}

// QueueSize returns the current number of members in the queue for itemID.
func (s *Store) QueueSize(ctx context.Context, itemID string) (int64, error) {
	return s.rdb.ZCard(ctx, prefixQueue+itemID).Result()
}
