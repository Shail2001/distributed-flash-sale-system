resource "aws_cloudwatch_log_group" "waiting_room" {
  name              = "/ecs/${var.project}/waiting-room"
  retention_in_days = 7
  tags              = { Name = "${var.project}-waiting-room-logs" }
}

resource "aws_cloudwatch_log_group" "flash_sale_api" {
  name              = "/ecs/${var.project}/flash-sale-api"
  retention_in_days = 7
  tags              = { Name = "${var.project}-api-logs" }
}

resource "aws_cloudwatch_log_group" "order_worker" {
  name              = "/ecs/${var.project}/order-worker"
  retention_in_days = 7
  tags              = { Name = "${var.project}-worker-logs" }
}
