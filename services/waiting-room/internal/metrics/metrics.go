package metrics

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Collector tracks waiting room counters using atomic primitives.
// Mirrors the same pattern used in the order-worker metrics package.
type Collector struct {
	joins       atomic.Uint64 // total POST /queue/join calls
	admitted    atomic.Uint64 // users whose position <= admitted_count at poll time
	outOfOrder  atomic.Uint64 // joins where assigned position > expected sequential position
	pollsServed atomic.Uint64 // total GET /queue/position calls
}

func (c *Collector) IncJoins()       { c.joins.Add(1) }
func (c *Collector) IncAdmitted()    { c.admitted.Add(1) }
func (c *Collector) IncOutOfOrder()  { c.outOfOrder.Add(1) }
func (c *Collector) IncPollsServed() { c.pollsServed.Add(1) }

func (c *Collector) Joins() uint64       { return c.joins.Load() }
func (c *Collector) Admitted() uint64    { return c.admitted.Load() }
func (c *Collector) OutOfOrder() uint64  { return c.outOfOrder.Load() }
func (c *Collector) PollsServed() uint64 { return c.pollsServed.Load() }

// OutOfOrderRate returns the fraction of joins that were out-of-order.
// Returns 0 if no joins have occurred yet.
func (c *Collector) OutOfOrderRate() float64 {
	joins := c.joins.Load()
	if joins == 0 {
		return 0
	}
	return float64(c.outOfOrder.Load()) / float64(joins)
}

// RunLogger emits periodic metrics logs until ctx is cancelled.
func (c *Collector) RunLogger(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastAdmitted := c.admitted.Load()
	lastTick := time.Now()

	for {
		select {
		case <-ctx.Done():
			logger.Info("metrics logger stopped",
				slog.Uint64("total_joins", c.joins.Load()),
				slog.Uint64("total_admitted", c.admitted.Load()),
				slog.Uint64("total_out_of_order", c.outOfOrder.Load()),
				slog.Float64("out_of_order_rate", c.OutOfOrderRate()),
				slog.Uint64("total_polls_served", c.pollsServed.Load()),
			)
			return
		case now := <-ticker.C:
			currentAdmitted := c.admitted.Load()
			elapsed := now.Sub(lastTick).Seconds()
			admitRate := 0.0
			if elapsed > 0 {
				admitRate = float64(currentAdmitted-lastAdmitted) / elapsed
			}

			logger.Info("waiting room metrics",
				slog.Uint64("total_joins", c.joins.Load()),
				slog.Uint64("total_admitted", currentAdmitted),
				slog.Uint64("total_out_of_order", c.outOfOrder.Load()),
				slog.Float64("out_of_order_rate", c.OutOfOrderRate()),
				slog.Float64("admit_rate_per_sec", admitRate),
				slog.Uint64("polls_served", c.pollsServed.Load()),
			)

			lastAdmitted = currentAdmitted
			lastTick = now
		}
	}
}
