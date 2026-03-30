# Dead Letter Queue
# Messages that fail maxReceiveCount times go here for inspection
resource "aws_sqs_queue" "dlq" {
  name                      = "${var.project}-orders-dlq"
  message_retention_seconds = 1209600 # 14 days - gives time to investigate failures

  tags = { Name = "${var.project}-orders-dlq" }
}

# Main Orders Queue
resource "aws_sqs_queue" "orders" {
  name                       = "${var.project}-orders"
  visibility_timeout_seconds = 30    # Must be > order worker processing time
  message_retention_seconds  = 86400 # 1 day
  receive_wait_time_seconds  = 20    # Long polling - reduces empty receives and cost

  # After 3 failed processing attempts, move to DLQ
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq.arn
    maxReceiveCount     = 3
  })

  tags = { Name = "${var.project}-orders" }
}

# Queue Policy (allows Flash Sale API to publish) 
resource "aws_sqs_queue_policy" "orders" {
  queue_url = aws_sqs_queue.orders.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = var.ecs_task_role_arn }
      Action    = ["sqs:SendMessage", "sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes"]
      Resource  = aws_sqs_queue.orders.arn
    }]
  })
}
