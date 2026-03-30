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

variable "waiting_room_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-waiting-room:latest"
}

variable "flash_sale_api_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-api:latest"
}

variable "order_worker_image" {
  type    = string
  default = "125878857213.dkr.ecr.us-east-1.amazonaws.com/flash-sale-order-worker:latest"
}

variable "inventory_count" {
  type        = number
  default     = 100
  description = "Number of items available in the flash sale"
}

variable "admission_rate" {
  type        = number
  default     = 10
  description = "Users admitted per second from waiting room (Experiment 3 sweep variable)"
}

variable "order_worker_goroutines" {
  type        = number
  default     = 20
  description = "Number of goroutines in order worker (Experiment 3 sweep variable)"
}
