package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

type permanentError struct {
	err error
}

func (e permanentError) Error() string {
	return e.err.Error()
}

func (e permanentError) Unwrap() error {
	return e.err
}

// Permanent marks an error as non-retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// Do retries fn with exponential backoff + full jitter until it succeeds,
// the context is canceled, or the maximum number of attempts is reached.
//
// Full jitter formula: sleep = rand(0, delay)
// This spreads retries uniformly across the backoff window, preventing
// retry storms when many goroutines fail simultaneously on the same
// DynamoDB item (TransactionConflict).
//
// Example with baseDelay=200ms:
//   attempt 1: sleep rand(0, 200ms)
//   attempt 2: sleep rand(0, 400ms)
//   attempt 3: sleep rand(0, 800ms)
func Do(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	var err error
	delay := baseDelay

	for attempt := 1; attempt <= attempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}

		var permanent permanentError
		if errors.As(err, &permanent) {
			return permanent.err
		}

		if attempt == attempts {
			return err
		}

		// Full jitter: sleep a random duration in [0, delay].
		// avoids the thundering herd problem when many workers
		// retry at the same instant after a TransactionConflict.
		jitter := time.Duration(rand.Int63n(int64(delay)))
		timer := time.NewTimer(jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		delay *= 2
	}

	return err
}
