variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "project" {
  type    = string
  default = "flash-sale"
}

variable "environment" {
  type    = string
  default = "dev"
}

variable "flash_sale_api_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-api:latest"
}

variable "order_worker_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-order-worker:latest"
}

variable "waiting_room_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-waiting-room:latest"
}

variable "inventory_count" {
  type        = number
  default     = 500
  description = "Number of items available in the flash sale"
}

variable "admission_rate" {
  type        = number
  default     = 10
  description = "Users admitted per second from waiting room (Experiment 3 sweep variable)"
}

variable "queue_strategy" {
  type        = string
  default     = "timestamp"
  description = "Waiting-room queue strategy for Experiment 1: timestamp or timestamp_incr"
}

variable "order_worker_goroutines" {
  type        = number
  default     = 20
  description = "Number of goroutines in order worker (Experiment 3 sweep variable)"
}

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
