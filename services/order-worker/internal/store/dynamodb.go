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

func (s *DynamoOrderStore) SaveOrder(ctx context.Context, order model.Order) (SaveResult, error) {
	now := s.now().UTC()
	record := order.ToRecord(now)

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return SaveResult{}, fmt.Errorf("marshal order record: %w", err)
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           aws.String(s.tableName),
					Item:                item,
					ConditionExpression: aws.String("attribute_not_exists(order_id)"),
				},
			},
			{
				Update: &types.Update{
					TableName: aws.String(s.inventoryTable),
					Key: map[string]types.AttributeValue{
						"item_id": &types.AttributeValueMemberS{Value: order.ItemID},
					},
					UpdateExpression: aws.String(
						"SET confirmed_orders = if_not_exists(confirmed_orders, :zero) + :inc, updated_at = :updated_at, last_order_id = :last_order_id",
					),
					ExpressionAttributeValues: map[string]types.AttributeValue{
						":zero":          &types.AttributeValueMemberN{Value: "0"},
						":inc":           &types.AttributeValueMemberN{Value: strconv.Itoa(order.Quantity)},
						":updated_at":    &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
						":last_order_id": &types.AttributeValueMemberS{Value: order.OrderID},
					},
				},
			},
		},
	})
	if err == nil {
		return SaveResult{Inserted: true}, nil
	}

	var txCanceled *types.TransactionCanceledException
	if errors.As(err, &txCanceled) && isDuplicateOrderCancellation(txCanceled) {
		return SaveResult{Inserted: false}, nil
	}

	var conditionalErr *types.ConditionalCheckFailedException
	if errors.As(err, &conditionalErr) {
		return SaveResult{Inserted: false}, nil
	}

	return SaveResult{}, fmt.Errorf("transact order write: %w", err)
}

func isDuplicateOrderCancellation(err *types.TransactionCanceledException) bool {
	if len(err.CancellationReasons) == 0 {
		return false
	}

	return aws.ToString(err.CancellationReasons[0].Code) == "ConditionalCheckFailed"
}
