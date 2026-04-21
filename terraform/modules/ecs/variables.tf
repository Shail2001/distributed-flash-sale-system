variable "project" { type = string }
variable "aws_region" { type = string }
variable "subnet_ids" { type = list(string) }
variable "ecs_security_group_id" { type = string }

# Container images
variable "flash_sale_api_image" { type = string }
variable "order_worker_image" { type = string }
variable "waiting_room_image" { type = string }

# ALB target groups
variable "flash_sale_api_target_group_arn" { type = string }
variable "waiting_room_target_group_arn" { type = string }

# Experiment variables — change these between runs, no rebuild needed
variable "inventory_count" {
  type        = number
  default     = 500
  description = "Starting inventory count"
}

variable "order_worker_goroutines" {
  type        = number
  default     = 20
  description = "NUM_WORKERS — Experiment 3 sweep: 20 / 40 / 80"
}

variable "admission_rate" {
  type        = number
  default     = 10
  description = "Admission rate/sec — Experiment 3 sweep: 2 → 100"
}

variable "queue_strategy" {
  type        = string
  default     = "timestamp"
  description = "QUEUE_STRATEGY for waiting-room Experiment 1"
}

# ECS desired task counts — scale up for experiments
variable "flash_sale_api_desired_count" {
  type        = number
  default     = 4
  description = "Number of Flash Sale API ECS tasks"
}

variable "order_worker_desired_count" {
  type        = number
  default     = 4
  description = "Number of Order Worker ECS tasks"
}

variable "waiting_room_desired_count" {
  type        = number
  default     = 2
  description = "Number of Waiting Room ECS tasks"
}

# Downstream resource references
variable "redis_endpoint" { type = string }
variable "sqs_queue_url" { type = string }
variable "sqs_queue_name" { type = string }
variable "dynamodb_orders_table" { type = string }
variable "dynamodb_inventory_table" { type = string }

# Log groups
variable "flash_sale_api_log_group" { type = string }
variable "order_worker_log_group" { type = string }
variable "waiting_room_log_group" { type = string }
