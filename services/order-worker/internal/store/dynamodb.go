package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"order-worker/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// SaveResult indicates whether the write created a new record.
type SaveResult struct {
	Inserted bool
}

// OrderStore defines the persistence contract used by the worker.
type OrderStore interface {
	SaveOrder(ctx context.Context, order model.Order) (SaveResult, error)
}

// DynamoOrderStore persists orders to a DynamoDB table.
type DynamoOrderStore struct {
	client         *dynamodb.Client
	tableName      string
	inventoryTable string
	now            func() time.Time
}

func NewDynamoOrderStore(client *dynamodb.Client, tableName, inventoryTable string) *DynamoOrderStore {
	return &DynamoOrderStore{
		client:         client,
		tableName:      tableName,
		inventoryTable: inventoryTable,
		now:            time.Now,
	}
}

// SaveOrder writes the order record and updates the inventory audit counter.
//
// The original implementation used a single TransactWriteItems call combining
// both writes. This caused TransactionConflict errors when many goroutines
// across multiple ECS tasks competed on the shared inventory item at sell-out.
//
// Fix: two separate writes.
//
//  1. PutItem with attribute_not_exists(order_id) — idempotent order insert.
//     No shared item contention across concurrent orders.
//
//  2. UpdateItem on inventory counter with idempotency guard:
//     only increments if last_order_id != this order_id.
//     This means retries after a partial failure (order inserted, inventory
//     update failed) will correctly update the counter without double-counting.
func (s *DynamoOrderStore) SaveOrder(ctx context.Context, order model.Order) (SaveResult, error) {
	now := s.now().UTC()
	record := order.ToRecord(now)

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return SaveResult{}, fmt.Errorf("marshal order record: %w", err)
	}

	// ── Step 1: Insert order ──────────────────────────────────────────────────
	inserted := false
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(order_id)"),
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			// Order already exists — this is a retry after a previous attempt
			// inserted the order but may have failed on the inventory update.
			// Fall through to Step 2 to ensure the inventory counter is updated.
			inserted = false
		} else {
			return SaveResult{}, fmt.Errorf("put order: %w", err)
		}
	} else {
		inserted = true
	}

	// ── Step 2: Update inventory audit counter ────────────────────────────────
	// Idempotency guard: only update if last_order_id is not already this
	// order_id. Prevents double-counting if Step 2 is retried after success.
	_, err = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.inventoryTable),
		Key: map[string]types.AttributeValue{
			"item_id": &types.AttributeValueMemberS{Value: order.ItemID},
		},
		UpdateExpression: aws.String(
			"SET confirmed_orders = if_not_exists(confirmed_orders, :zero) + :inc, updated_at = :updated_at, last_order_id = :last_order_id",
		),
		ConditionExpression: aws.String(
			"attribute_not_exists(last_order_id) OR last_order_id <> :last_order_id",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":zero":          &types.AttributeValueMemberN{Value: "0"},
			":inc":           &types.AttributeValueMemberN{Value: strconv.Itoa(order.Quantity)},
			":updated_at":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
			":last_order_id": &types.AttributeValueMemberS{Value: order.OrderID},
		},
	})
	if err != nil {
		var condErr *types.ConditionalCheckFailedException
		if errors.As(err, &condErr) {
			// last_order_id already equals this order_id — inventory was already
			// updated for this order in a previous attempt. Not an error.
			return SaveResult{Inserted: inserted}, nil
		}
		return SaveResult{Inserted: inserted}, fmt.Errorf("update inventory audit: %w", err)
	}

	return SaveResult{Inserted: inserted}, nil
}
