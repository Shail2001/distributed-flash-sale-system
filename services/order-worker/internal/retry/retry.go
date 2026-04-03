package retry

import (
	"context"
	"errors"
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

// Do retries fn with exponential backoff until it succeeds, the context is canceled,
// or the maximum number of attempts is reached.
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

		timer := time.NewTimer(delay)
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
