package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultNumWorkers            = 10
	defaultWaitTimeSeconds       = 20
	defaultMaxMessagesPerReceive = 10
	defaultVisibilityTimeoutSecs = 60
	defaultMetricsInterval       = 30 * time.Second
	defaultRetryAttempts         = 5                  // increased from 3 — handles TransactionConflict bursts
	defaultRetryBaseDelay        = 100 * time.Millisecond // reduced from 200ms — jitter spreads retries, base can be tighter
	defaultIdleSleep             = 500 * time.Millisecond
)

// Config contains all runtime configuration for the worker.
type Config struct {
	AWSRegion                string
	SQSQueueURL              string
	DynamoDBTable            string
	InventoryTable           string
	NumWorkers               int
	WaitTimeSeconds          int32
	MaxMessagesPerReceive    int32
	VisibilityTimeoutSeconds int32
	MetricsInterval          time.Duration
	RetryAttempts            int
	RetryBaseDelay           time.Duration
	IdleSleep                time.Duration
}

// Load reads configuration from environment variables and applies safe defaults.
func Load() (Config, error) {
	cfg := Config{
		AWSRegion:                os.Getenv("AWS_REGION"),
		SQSQueueURL:              os.Getenv("SQS_QUEUE_URL"),
		DynamoDBTable:            os.Getenv("DYNAMODB_TABLE"),
		InventoryTable:           os.Getenv("INVENTORY_TABLE"),
		NumWorkers:               intFromEnv("NUM_WORKERS", defaultNumWorkers),
		WaitTimeSeconds:          defaultWaitTimeSeconds,
		MaxMessagesPerReceive:    defaultMaxMessagesPerReceive,
		VisibilityTimeoutSeconds: defaultVisibilityTimeoutSecs,
		MetricsInterval:          defaultMetricsInterval,
		RetryAttempts:            defaultRetryAttempts,
		RetryBaseDelay:           defaultRetryBaseDelay,
		IdleSleep:                defaultIdleSleep,
	}

	switch {
	case cfg.AWSRegion == "":
		return Config{}, fmt.Errorf("AWS_REGION is required")
	case cfg.SQSQueueURL == "":
		return Config{}, fmt.Errorf("SQS_QUEUE_URL is required")
	case cfg.DynamoDBTable == "":
		return Config{}, fmt.Errorf("DYNAMODB_TABLE is required")
	case cfg.InventoryTable == "":
		return Config{}, fmt.Errorf("INVENTORY_TABLE is required")
	case cfg.NumWorkers < 1:
		return Config{}, fmt.Errorf("NUM_WORKERS must be greater than 0")
	}

	return cfg, nil
}

func intFromEnv(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}

	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}

	return n
}
