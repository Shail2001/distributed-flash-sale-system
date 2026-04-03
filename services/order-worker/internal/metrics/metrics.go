package metrics

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Collector tracks high-level processing counters using atomic primitives.
type Collector struct {
	confirmed  atomic.Uint64
	duplicates atomic.Uint64
	failed     atomic.Uint64
}

func (c *Collector) IncConfirmed() {
	c.confirmed.Add(1)
}

func (c *Collector) IncDuplicate() {
	c.duplicates.Add(1)
}

func (c *Collector) IncFailed() {
	c.failed.Add(1)
}

func (c *Collector) Confirmed() uint64 {
	return c.confirmed.Load()
}

// RunLogger emits periodic metrics and throughput logs.
func (c *Collector) RunLogger(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastConfirmed := c.confirmed.Load()
	lastTick := time.Now()

	for {
		select {
		case <-ctx.Done():
			logger.Info("metrics logger stopped",
				slog.Uint64("confirmed_orders", c.confirmed.Load()),
				slog.Uint64("duplicates", c.duplicates.Load()),
				slog.Uint64("failed", c.failed.Load()),
			)
			return
		case now := <-ticker.C:
			currentConfirmed := c.confirmed.Load()
			elapsed := now.Sub(lastTick).Seconds()
			rate := 0.0
			if elapsed > 0 {
				rate = float64(currentConfirmed-lastConfirmed) / elapsed
			}

			logger.Info("worker metrics",
				slog.Uint64("confirmed_orders", currentConfirmed),
				slog.Uint64("duplicates", c.duplicates.Load()),
				slog.Uint64("failed", c.failed.Load()),
				slog.Float64("processed_per_sec", rate),
			)

			lastConfirmed = currentConfirmed
			lastTick = now
		}
	}
}
