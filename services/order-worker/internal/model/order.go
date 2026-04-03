package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const orderTTL = 90 * 24 * time.Hour

// Order represents a purchase accepted by the flash-sale API.
// The worker accepts both user_id and customer_id for compatibility.
type Order struct {
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	ItemID    string    `json:"item_id"`
	Quantity  int       `json:"quantity"`
	Timestamp time.Time `json:"-"`
	Strategy  string    `json:"strategy,omitempty"`
}

// OrderRecord is the DynamoDB representation persisted for each order.
type OrderRecord struct {
	OrderID     string `dynamodbav:"order_id"`
	UserID      string `dynamodbav:"user_id"`
	CustomerID  string `dynamodbav:"customer_id"`
	ItemID      string `dynamodbav:"item_id"`
	Quantity    int    `dynamodbav:"quantity"`
	Timestamp   string `dynamodbav:"timestamp"`
	Strategy    string `dynamodbav:"strategy,omitempty"`
	ProcessedAt string `dynamodbav:"processed_at"`
	Status      string `dynamodbav:"status"`
	ExpiresAt   int64  `dynamodbav:"expires_at"`
}

type rawOrder struct {
	OrderID    string `json:"order_id"`
	UserID     string `json:"user_id"`
	CustomerID string `json:"customer_id"`
	ItemID     string `json:"item_id"`
	Quantity   int    `json:"quantity"`
	Timestamp  string `json:"timestamp"`
	Strategy   string `json:"strategy"`
}

// UnmarshalJSON allows the worker to consume both user_id and customer_id payloads.
func (o *Order) UnmarshalJSON(data []byte) error {
	var raw rawOrder
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	userID := raw.UserID
	if userID == "" {
		userID = raw.CustomerID
	}

	parsedTime, err := time.Parse(time.RFC3339, raw.Timestamp)
	if err != nil {
		return fmt.Errorf("parse timestamp: %w", err)
	}

	o.OrderID = raw.OrderID
	o.UserID = userID
	o.ItemID = raw.ItemID
	o.Quantity = raw.Quantity
	o.Timestamp = parsedTime.UTC()
	o.Strategy = raw.Strategy

	return nil
}

// Validate performs basic sanity checks before processing.
func (o Order) Validate() error {
	switch {
	case strings.TrimSpace(o.OrderID) == "":
		return fmt.Errorf("order_id is required")
	case strings.TrimSpace(o.UserID) == "":
		return fmt.Errorf("user_id is required")
	case strings.TrimSpace(o.ItemID) == "":
		return fmt.Errorf("item_id is required")
	case o.Quantity < 1:
		return fmt.Errorf("quantity must be greater than 0")
	case o.Timestamp.IsZero():
		return fmt.Errorf("timestamp is required")
	}

	return nil
}

// ToRecord converts an order into the persisted representation.
func (o Order) ToRecord(now time.Time) OrderRecord {
	return OrderRecord{
		OrderID:     o.OrderID,
		UserID:      o.UserID,
		CustomerID:  o.UserID,
		ItemID:      o.ItemID,
		Quantity:    o.Quantity,
		Timestamp:   o.Timestamp.UTC().Format(time.RFC3339),
		Strategy:    o.Strategy,
		ProcessedAt: now.UTC().Format(time.RFC3339),
		Status:      "confirmed",
		ExpiresAt:   now.UTC().Add(orderTTL).Unix(),
	}
}
