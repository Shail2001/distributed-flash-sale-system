package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"waiting-room/internal/admission"
	"waiting-room/internal/config"
	"waiting-room/internal/handlers"
	"waiting-room/internal/metrics"
	"waiting-room/internal/queue"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}

	// PoolSize + PoolTimeout tuned for 10k-concurrent-joiner bursts. The go-redis
	// default (PoolSize = 10*NumCPU ≈ 80; PoolTimeout = 4s) saturated under that
	// load: peak instantaneous connection demand exceeded 500 during the join
	// storm, and the admission ticker's metric INCR lost connection-wait races
	// and returned "redis: connection pool timeout". 2000 slots + 10s wait
	// absorb the burst; steady-state connection count is far lower.
	rdb := goredis.NewClient(&goredis.Options{
		Addr:        fmt.Sprintf("%s:%s", cfg.RedisEndpoint, cfg.RedisPort),
		PoolSize:    2000,
		PoolTimeout: 10 * time.Second,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := queue.NewStore(rdb)
	collector := &metrics.Collector{}

	// Background: admit users at ADMISSION_RATE per second.
	ticker := admission.New(store, cfg.ItemID, cfg.AdmissionRate, logger)
	go ticker.Run(ctx)

	// Background: emit metrics every 30 seconds.
	go collector.RunLogger(ctx, logger, 30*time.Second)

	h := handlers.NewHandler(store, cfg.QueueStrategy, cfg.ItemID, collector)

	r := gin.Default()
	r.GET("/health", h.Health)
	r.POST("/queue/join", h.Join)
	r.GET("/queue/position", h.Position)
	r.POST("/queue/reset", h.Reset)

	logger.Info("waiting room starting",
		slog.String("port", cfg.AppPort),
		slog.String("strategy", cfg.QueueStrategy),
		slog.Int("admission_rate", cfg.AdmissionRate),
		slog.String("item_id", cfg.ItemID),
	)

	if err := r.Run(fmt.Sprintf(":%s", cfg.AppPort)); err != nil {
		logger.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	}
}
