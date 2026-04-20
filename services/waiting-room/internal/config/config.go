package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	StrategyTimestamp     = "timestamp"
	StrategyTimestampIncr = "timestamp_incr"
)

// Config holds all runtime configuration for the waiting room service.
type Config struct {
	RedisEndpoint string
	RedisPort     string
	AdmissionRate int    // users admitted per second — Experiment 3 sweep variable
	QueueStrategy string // "timestamp" or "timestamp_incr" — Experiment 1 switch
	AppPort       string
	ItemID        string
}

// Load reads configuration from environment variables and applies safe defaults.
func Load() (Config, error) {
	cfg := Config{
		RedisEndpoint: getEnv("REDIS_ENDPOINT", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		AdmissionRate: intFromEnv("ADMISSION_RATE", 10),
		QueueStrategy: getEnv("QUEUE_STRATEGY", StrategyTimestamp),
		AppPort:       getEnv("APP_PORT", "8080"),
		ItemID:        getEnv("ITEM_ID", "flash-sale-item"),
	}

	switch {
	case cfg.AdmissionRate < 1:
		return Config{}, fmt.Errorf("ADMISSION_RATE must be >= 1, got %d", cfg.AdmissionRate)
	case cfg.QueueStrategy != StrategyTimestamp && cfg.QueueStrategy != StrategyTimestampIncr:
		return Config{}, fmt.Errorf("QUEUE_STRATEGY must be %q or %q, got %q",
			StrategyTimestamp, StrategyTimestampIncr, cfg.QueueStrategy)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intFromEnv(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
