package admission

import (
	"context"
	"log/slog"
	"time"

	"waiting-room/internal/queue"
)

// Ticker releases users from the waiting room into the Flash Sale at a fixed rate.
// Each tick increments the admitted counter in Redis by 1.
// Users whose queue position <= admitted count are given an admission token.
type Ticker struct {
	store  *queue.Store
	itemID string
	rate   int // users admitted per second — ADMISSION_RATE env var
	logger *slog.Logger
}

func New(store *queue.Store, itemID string, rate int, logger *slog.Logger) *Ticker {
	return &Ticker{store: store, itemID: itemID, rate: rate, logger: logger}
}

// Run blocks until ctx is cancelled, incrementing the admitted counter at the configured rate.
func (t *Ticker) Run(ctx context.Context) {
	interval := time.Second / time.Duration(t.rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	t.logger.Info("admission ticker started",
		slog.Int("rate_per_sec", t.rate),
		slog.Duration("tick_interval", interval),
	)

	for {
		select {
		case <-ctx.Done():
			t.logger.Info("admission ticker stopped")
			return
		case <-ticker.C:
			if err := t.store.IncrAdmitted(ctx, t.itemID, 1); err != nil {
				// Don't log on context cancellation — that's a clean shutdown.
				if ctx.Err() == nil {
					t.logger.Error("failed to increment admitted count", slog.Any("error", err))
				}
			}
		}
	}
}
