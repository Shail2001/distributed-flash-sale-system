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

// joinTimestampScript: ZADD (NX) at the client-supplied ms timestamp, then ZRANK.
// Strategy A for Exp 1 — ties are possible since multiple clients can land in the
// same ms bucket. ZADD and ZRANK run atomically on the server, so the caller's
// rank is a true snapshot of the queue at ADD time (not a later state that may
// have shifted under concurrent inserts).
// KEYS[1] = queue key, ARGV[1] = score, ARGV[2] = member id.
var joinTimestampScript = redis.NewScript(`
	redis.call('ZADD', KEYS[1], 'NX', ARGV[1], ARGV[2])
	return redis.call('ZRANK', KEYS[1], ARGV[2])
`)

// joinIncrScript: INCR sequence, ZADD (NX) at that score, ZRANK — all atomic.
// Strategy B for Exp 1. Unlike the earlier ms + seq/1e9 attempt, seq is used as
// the entire score, so it is exact in float64 for realistic queue sizes and the
// INCR cannot be reordered relative to the ZADD by concurrent callers.
// KEYS[1] = queue key, KEYS[2] = seq key, ARGV[1] = member id.
var joinIncrScript = redis.NewScript(`
	local seq = redis.call('INCR', KEYS[2])
	redis.call('ZADD', KEYS[1], 'NX', seq, ARGV[1])
	return redis.call('ZRANK', KEYS[1], ARGV[1])
`)

// Join adds userID to the queue for itemID using the given strategy.
// Idempotent: if userID is already present, returns their existing position unchanged.
// Returns 1-indexed position.
func (s *Store) Join(ctx context.Context, userID, itemID, strategy string) (int64, error) {
	queueKey := prefixQueue + itemID

	// Already in queue — return existing rank without changing it.
	if rank, err := s.rdb.ZRank(ctx, queueKey, userID).Result(); err == nil {
		return rank + 1, nil
	}

	var rank int64
	var err error
	if strategy == "timestamp_incr" {
		rank, err = joinIncrScript.Run(
			ctx, s.rdb,
			[]string{queueKey, prefixSeq + itemID},
			userID,
		).Int64()
	} else {
		rank, err = joinTimestampScript.Run(
			ctx, s.rdb,
			[]string{queueKey},
			time.Now().UnixMilli(),
			userID,
		).Int64()
	}
	if err != nil {
		return 0, fmt.Errorf("join script: %w", err)
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


// QueueSize returns the current number of members in the queue for itemID.
func (s *Store) QueueSize(ctx context.Context, itemID string) (int64, error) {
	return s.rdb.ZCard(ctx, prefixQueue+itemID).Result()
}
