#!/usr/bin/env bash
set -euo pipefail

# Bootstrap LocalStack: creates the SQS queue and DynamoDB tables the services expect.
# Assumes docker-compose.local.yml is already up and LocalStack is healthy.

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-test}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-test}"
ENDPOINT="${AWS_ENDPOINT_URL:-http://localhost:4566}"

QUEUE_NAME="${QUEUE_NAME:-flash-sale-orders}"
ORDERS_TABLE="${DYNAMODB_TABLE:-flash-sale-orders}"
INVENTORY_TABLE="${INVENTORY_TABLE:-flash-sale-inventory}"

echo "==> Creating SQS queue: $QUEUE_NAME"
aws --endpoint-url "$ENDPOINT" sqs create-queue \
  --queue-name "$QUEUE_NAME" \
  --attributes VisibilityTimeout=60 \
  >/dev/null

QUEUE_URL=$(aws --endpoint-url "$ENDPOINT" sqs get-queue-url --queue-name "$QUEUE_NAME" --output text --query QueueUrl)
echo "    queue url: $QUEUE_URL"

echo "==> Creating DynamoDB table: $ORDERS_TABLE (orders)"
aws --endpoint-url "$ENDPOINT" dynamodb create-table \
  --table-name "$ORDERS_TABLE" \
  --attribute-definitions AttributeName=order_id,AttributeType=S \
  --key-schema AttributeName=order_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  2>/dev/null || echo "    (already exists)"

echo "==> Creating DynamoDB table: $INVENTORY_TABLE (inventory audit)"
aws --endpoint-url "$ENDPOINT" dynamodb create-table \
  --table-name "$INVENTORY_TABLE" \
  --attribute-definitions AttributeName=item_id,AttributeType=S \
  --key-schema AttributeName=item_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  2>/dev/null || echo "    (already exists)"

echo "==> Waiting for tables ACTIVE"
aws --endpoint-url "$ENDPOINT" dynamodb wait table-exists --table-name "$ORDERS_TABLE"
aws --endpoint-url "$ENDPOINT" dynamodb wait table-exists --table-name "$INVENTORY_TABLE"

echo "==> Done. Resources:"
aws --endpoint-url "$ENDPOINT" sqs list-queues --output text
aws --endpoint-url "$ENDPOINT" dynamodb list-tables --output text
