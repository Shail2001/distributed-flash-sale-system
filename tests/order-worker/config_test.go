package orderworkertests

import (
	"testing"

	appconfig "order-worker/internal/config"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("SQS_QUEUE_URL", "https://example.com/queue")
	t.Setenv("DYNAMODB_TABLE", "orders")
	t.Setenv("INVENTORY_TABLE", "inventory")
	t.Setenv("NUM_WORKERS", "")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.NumWorkers != 10 {
		t.Fatalf("expected default workers 10, got %d", cfg.NumWorkers)
	}
}

func TestLoadRejectsMissingRequiredEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("SQS_QUEUE_URL", "https://example.com/queue")
	t.Setenv("DYNAMODB_TABLE", "orders")
	t.Setenv("INVENTORY_TABLE", "inventory")

	if _, err := appconfig.Load(); err == nil {
		t.Fatal("expected error for missing AWS_REGION")
	}
}
