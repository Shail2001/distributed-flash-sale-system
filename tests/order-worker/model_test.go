package orderworkertests

import (
	"encoding/json"
	"testing"
	"time"

	"order-worker/internal/model"
)

func TestOrderUnmarshalAcceptsCustomerID(t *testing.T) {
	payload := []byte(`{
		"order_id":"o-123",
		"customer_id":"user-1",
		"item_id":"flash-sale-item",
		"quantity":1,
		"timestamp":"2026-04-03T12:00:00Z"
	}`)

	var order model.Order
	if err := json.Unmarshal(payload, &order); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if order.UserID != "user-1" {
		t.Fatalf("expected user_id to be derived from customer_id, got %q", order.UserID)
	}
}

func TestOrderValidateRejectsMissingFields(t *testing.T) {
	order := model.Order{}
	if err := order.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestOrderToRecordMatchesTableShape(t *testing.T) {
	order := model.Order{
		OrderID:   "o-123",
		UserID:    "user-1",
		ItemID:    "flash-sale-item",
		Quantity:  2,
		Timestamp: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
		Strategy:  "atomic_decr",
	}

	record := order.ToRecord(time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC))
	if record.CustomerID != order.UserID {
		t.Fatalf("expected customer_id %q, got %q", order.UserID, record.CustomerID)
	}
	if record.ExpiresAt == 0 {
		t.Fatal("expected expires_at to be populated for TTL")
	}
}
