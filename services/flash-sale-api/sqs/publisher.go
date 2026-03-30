package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// OrderMessage is the JSON payload published to SQS for each accepted purchase.
type OrderMessage struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	Quantity   int    `json:"quantity"`
	ItemID     string `json:"item_id"`
	Timestamp  string `json:"timestamp"`
	Strategy   string `json:"strategy"`
}

// Publisher wraps the SQS client and queue URL.
type Publisher struct {
	client   *sqs.Client
	queueURL string
}

// NewPublisher creates a Publisher with the given SQS client and queue URL.
func NewPublisher(client *sqs.Client, queueURL string) *Publisher {
	return &Publisher{client: client, queueURL: queueURL}
}

// PublishOrder serializes an OrderMessage and sends it to the SQS queue.
func (p *Publisher) PublishOrder(ctx context.Context, orderID, customerID, itemID string, quantity int) error {
	msg := OrderMessage{
		OrderID:    orderID,
		CustomerID: customerID,
		Quantity:   quantity,
		ItemID:     itemID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Strategy:   "atomic_decr", // identifies this strategy for Experiment 2 comparisons
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal order message: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("failed to send SQS message: %w", err)
	}

	return nil
}
