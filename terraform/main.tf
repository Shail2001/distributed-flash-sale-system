# Distributed Flash Sale Platform
# CS6650 Final Project
#
# Cost optimisation: NAT Gateway removed — all services run in public subnets.
# Security groups enforce access control instead of subnet placement.
# Saves ~$35/month vs private subnet + NAT gateway setup.
#
# Experiment sweep variables (no container rebuild needed):
#   var.inventory_count          — fixed at 100
#   var.order_worker_goroutines  — Experiment 3: sweep 20/40/80
#   var.admission_rate           — Experiment 3: sweep 2→100/s

module "network" {
  source  = "./modules/network"
  project = var.project
}

module "ecr" {
  source  = "./modules/ecr"
  project = var.project
}

module "logging" {
  source  = "./modules/logging"
  project = var.project
}

module "redis" {
  source                  = "./modules/redis"
  project                 = var.project
  subnet_ids              = module.network.public_subnet_ids
  redis_security_group_id = module.network.redis_security_group_id
}

module "sqs" {
  source            = "./modules/sqs"
  project           = var.project
  ecs_task_role_arn = "arn:aws:iam::125878857213:role/LabRole"
}

module "dynamodb" {
  source  = "./modules/dynamodb"
  project = var.project
}

module "alb" {
  source                = "./modules/alb"
  project               = var.project
  vpc_id                = module.network.vpc_id
  public_subnet_ids     = module.network.public_subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
}

module "ecs" {
  source = "./modules/ecs"

  project    = var.project
  aws_region = var.aws_region

  subnet_ids            = module.network.public_subnet_ids
  ecs_security_group_id = module.network.ecs_security_group_id

  flash_sale_api_image = "${module.ecr.flash_sale_api_repo_url}:latest"
  order_worker_image   = "${module.ecr.order_worker_repo_url}:latest"
  waiting_room_image = "${module.ecr.waiting_room_repo_url}:latest"

  flash_sale_api_target_group_arn = module.alb.flash_sale_api_target_group_arn
  waiting_room_target_group_arn = module.alb.waiting_room_target_group_arn

  inventory_count         = var.inventory_count
  order_worker_goroutines = var.order_worker_goroutines
  admission_rate          = var.admission_rate

  redis_endpoint           = module.redis.redis_endpoint
  sqs_queue_url            = module.sqs.orders_queue_url
  sqs_queue_name           = "${var.project}-orders"
  dynamodb_orders_table    = module.dynamodb.orders_table_name
  dynamodb_inventory_table = module.dynamodb.inventory_table_name

  flash_sale_api_log_group = module.logging.flash_sale_api_log_group
  order_worker_log_group   = module.logging.order_worker_log_group
  waiting_room_log_group = module.logging.waiting_room_log_group
}
