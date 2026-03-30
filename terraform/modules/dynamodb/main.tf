# Orders Table
# Stores confirmed orders after Order Worker processes from SQS
# On-demand billing: pay per request, no minimum - best for bursty flash sale workload
resource "aws_dynamodb_table" "orders" {
  name         = "${var.project}-orders"
  billing_mode = "PAY_PER_REQUEST" # On-demand - cheaper than provisioned for bursty traffic

  # Partition key: order_id (UUID) - ensures even distribution across partitions
  hash_key = "order_id"

  attribute {
    name = "order_id"
    type = "S"
  }

  # GSI: query all orders by customer_id
  attribute {
    name = "customer_id"
    type = "S"
  }

  global_secondary_index {
    name            = "customer-index"
    hash_key        = "customer_id"
    projection_type = "ALL"
  }

  # TTL: auto-expire old orders after 90 days to control storage costs
  ttl {
    attribute_name = "expires_at"
    enabled        = true
  }

  tags = { Name = "${var.project}-orders" }
}

# Inventory Table
# Source of truth for inventory count - used for post-experiment consistency verification
# Redis is the fast path; DynamoDB is the audit trail
resource "aws_dynamodb_table" "inventory" {
  name         = "${var.project}-inventory"
  billing_mode = "PAY_PER_REQUEST"

  hash_key = "item_id"

  attribute {
    name = "item_id"
    type = "S"
  }

  tags = { Name = "${var.project}-inventory" }
}
