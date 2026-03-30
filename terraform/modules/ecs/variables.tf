variable "project" { type = string }
variable "aws_region" { type = string }
variable "private_subnet_ids" { type = list(string) }
variable "ecs_security_group_id" { type = string }

# Images
variable "waiting_room_image" { type = string }
variable "flash_sale_api_image" { type = string }
variable "order_worker_image" { type = string }

# ALB
variable "waiting_room_target_group_arn" { type = string }
variable "flash_sale_api_target_group_arn" { type = string }
variable "alb_listener_arn" { type = string }

# Config - these are the Experiment 3 sweep variables
variable "admission_rate" {
  type        = number
  default     = 10
  description = "Users admitted per second from waiting room"
}

variable "inventory_count" {
  type        = number
  default     = 100
  description = "Starting inventory for flash sale"
}

variable "order_worker_goroutines" {
  type        = number
  default     = 20
  description = "Goroutines in order worker"
}

# Downstream resource references
variable "redis_endpoint" { type = string }
variable "sqs_queue_url" { type = string }
variable "sqs_queue_name" { type = string }
variable "dynamodb_orders_table" { type = string }
variable "dynamodb_inventory_table" { type = string }

# Log groups
variable "waiting_room_log_group" { type = string }
variable "flash_sale_api_log_group" { type = string }
variable "order_worker_log_group" { type = string }
